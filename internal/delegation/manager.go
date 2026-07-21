// Package delegation mints and caches per-scope Zitadel machine users
// ("delegates") and hands out Zitadel clients authenticated as them.
//
// v0.18 (INF-425). Per resolved scope:
//   - explicit machine-user create (never relies on ORG_PROJECT_CREATOR
//     self-grant — zitadel#10561: the creator role does not own the project
//     it creates)
//   - explicit membership grant: org-scope => ORG_OWNER org member,
//     project-scope => PROJECT_OWNER member on the project (project created
//     if missing, using the binding credential)
//   - key JSON cached in a labeled Secret zitadel-delegation-<scope-hash>
//     in the operator namespace, persisted BEFORE the in-memory cache so a
//     crash cannot lose a minted key; restart warms from those Secrets
//   - lazy validity re-check on first warm use (out-of-band SA deletion
//     => re-mint)
//   - eager revoke when a scope stops matching + periodic orphan GC
//   - internal dual-key rotation on a 90-day cycle (mint new -> swap Secret
//     -> revoke old after grace)
package delegation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/metadata"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/authn"
	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	objectv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
	userv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user"
)

const (
	// DelegationLabel marks Secrets holding delegated credentials.
	DelegationLabel = "zitadel.truvity.io/delegation"

	// SecretNamePrefix + scope hash = delegation Secret name.
	SecretNamePrefix = "zitadel-delegation-" //nolint:gosec // G101: Secret name prefix, not a credential

	// DefaultUsernamePrefix is the delegate machine-user username prefix.
	DefaultUsernamePrefix = "zitadel-operator-delegate-"

	// DefaultRotateAfter is the delegate key rotation cycle.
	DefaultRotateAfter = 90 * 24 * time.Hour

	// DefaultRotationGrace is how long the old key stays valid after a new
	// key is swapped in (zero-downtime overlap).
	DefaultRotationGrace = 5 * time.Minute

	// keyExpiry is the validity of minted delegate keys. Rotation replaces
	// keys long before expiry; the expiry is a backstop.
	keyExpiry = 365 * 24 * time.Hour

	keyDataKey            = "key.json"
	scopeDataKey          = "scope.json"
	userIDDataKey         = "user_id"
	orgIDDataKey          = "org_id"
	projectIDDataKey      = "project_id"
	keyIDDataKey          = "key_id"
	keyCreatedDataKey     = "key_created"
	oldKeyIDDataKey       = "old_key_id"
	oldKeyRevokeAtDataKey = "old_key_revoke_after"
)

// Delegate is a minted per-scope identity plus a client acting as it.
type Delegate struct {
	// Client is a Zitadel client authenticated as the delegated machine user.
	Client *zitadel.Client
	// UserID is the delegated machine user's Zitadel ID.
	UserID string
	// OrgID is the delegate's organization (v1 Management API calls against
	// the delegate must carry it as x-zitadel-orgid).
	OrgID string
	// ProjectID is the resolved project ID (empty for org-scope).
	ProjectID string
	// KeyID is the current machine key ID.
	KeyID string
	// KeyCreated is when the current key was minted (rotation clock).
	KeyCreated time.Time
	// validated is true once the delegate's existence was re-checked against
	// Zitadel after a warm start (lazy re-mint trigger).
	validated bool
}

// Manager mints, persists, rotates and revokes delegates.
type Manager struct {
	// K8s writes delegation Secrets (operator namespace).
	K8s client.Client
	// Binding is the operator's own (high-privilege) Zitadel client,
	// used ONLY to mint/rotate/revoke delegates, never to reconcile tenant
	// resources once a scope is delegated.
	Binding *zitadel.Client
	// ClientCfg is the connection template for delegated clients
	// (KeyJSON is replaced per delegate).
	ClientCfg zitadel.ClientConfig
	// Namespace is the operator namespace holding delegation Secrets.
	Namespace string
	// UsernamePrefix overrides DefaultUsernamePrefix (tests use a
	// recognizable prefix on the shared test instance).
	UsernamePrefix string
	// RotateAfter overrides DefaultRotateAfter.
	RotateAfter time.Duration
	// RotationGrace overrides DefaultRotationGrace.
	RotationGrace time.Duration

	mu    sync.Mutex
	cache map[string]*Delegate // scope hash -> delegate
}

// withOrg appends the x-zitadel-orgid header when the org is known.
func withOrg(ctx context.Context, orgID string) context.Context {
	if orgID == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)
}

func (m *Manager) usernamePrefix() string {
	if m.UsernamePrefix != "" {
		return m.UsernamePrefix
	}
	return DefaultUsernamePrefix
}

func (m *Manager) rotateAfter() time.Duration {
	if m.RotateAfter > 0 {
		return m.RotateAfter
	}
	return DefaultRotateAfter
}

func (m *Manager) rotationGrace() time.Duration {
	if m.RotationGrace > 0 {
		return m.RotationGrace
	}
	return DefaultRotationGrace
}

// Ensure returns a delegate for the scope, minting it if needed.
// Order: in-memory cache -> Secret (warm restart) -> mint against Zitadel.
// Warm-start delegates are validated against Zitadel once (lazy re-mint when
// the SA was deleted out-of-band); keys past the rotation cycle are rotated.
func (m *Manager) Ensure(ctx context.Context, scope *scopemap.Scope) (*Delegate, error) {
	hash := scope.Hash()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cache == nil {
		m.cache = map[string]*Delegate{}
	}
	if d, ok := m.cache[hash]; ok {
		if err := m.validateLocked(ctx, hash, d); err != nil {
			return nil, err
		}
		if d2, ok2 := m.cache[hash]; ok2 && d2 != nil {
			return m.rotateIfDueLocked(ctx, hash, d2)
		}
		// validateLocked dropped a stale entry — fall through to re-mint.
	}

	// Warm from Secret (restart path); a stale Secret falls through to mint.
	if d, handled, err := m.warmLocked(ctx, hash); err != nil {
		return nil, err
	} else if handled {
		return d, nil
	}

	// Mint.
	d, keyJSON, err := m.mint(ctx, scope, hash)
	if err != nil {
		return nil, err
	}

	// Persist before caching so a crash cannot lose the only copy of the key.
	if err := m.persistSecret(ctx, scope, hash, keyJSON, d); err != nil {
		return nil, fmt.Errorf("persisting delegation Secret: %w", err)
	}

	m.cache[hash] = d
	return d, nil
}

// warmLocked tries to satisfy Ensure from the persisted Secret. handled=true
// means the delegate was warmed (and validated/rotated); handled=false means
// the caller must mint.
func (m *Manager) warmLocked(ctx context.Context, hash string) (d *Delegate, handled bool, err error) {
	var secret corev1.Secret
	err = m.K8s.Get(ctx, apitypes.NamespacedName{Name: SecretNamePrefix + hash, Namespace: m.Namespace}, &secret)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, false, fmt.Errorf("reading delegation Secret: %w", err)
		}
		return nil, false, nil
	}
	if len(secret.Data[keyDataKey]) == 0 {
		return nil, false, nil
	}
	wd, buildErr := m.delegateFromSecret(ctx, &secret)
	if buildErr != nil {
		return nil, false, fmt.Errorf("building delegated client from Secret %s: %w", secret.Name, buildErr)
	}
	m.cache[hash] = wd
	if err := m.validateLocked(ctx, hash, wd); err != nil {
		return nil, false, err
	}
	d2, ok := m.cache[hash]
	if !ok || d2 == nil {
		// Stale Secret (SA deleted out-of-band) — mint fresh.
		return nil, false, nil
	}
	log.FromContext(ctx).Info("delegation warmed from Secret", "scopeHash", hash, "userId", d2.UserID)
	rd, rerr := m.rotateIfDueLocked(ctx, hash, d2)
	return rd, true, rerr
}

// validateLocked re-checks a warm delegate against Zitadel exactly once.
// If the machine user no longer exists (deleted out-of-band), the stale
// Secret and cache entry are dropped so the caller re-mints.
func (m *Manager) validateLocked(ctx context.Context, hash string, d *Delegate) error {
	if d.validated {
		return nil
	}
	orgCtx := withOrg(ctx, d.OrgID)
	_, err := m.Binding.Management().GetUserByID(orgCtx, &management.GetUserByIDRequest{Id: d.UserID}) //nolint:staticcheck // SA1019: v1 Management API
	if err == nil {
		d.validated = true
		return nil
	}
	if status.Code(err) != codes.NotFound {
		return fmt.Errorf("validating delegate %s: %w", d.UserID, err)
	}
	log.FromContext(ctx).Info("delegate vanished out-of-band, dropping cached credential for re-mint",
		"scopeHash", hash, "userId", d.UserID)
	delete(m.cache, hash)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: SecretNamePrefix + hash, Namespace: m.Namespace}}
	delErr := m.K8s.Delete(ctx, secret)
	if client.IgnoreNotFound(delErr) != nil {
		return fmt.Errorf("deleting stale delegation Secret: %w", delErr)
	}
	return nil
}

// rotateIfDueLocked rotates the delegate key when it is past the rotation
// cycle: mint new key -> swap in Secret (old key recorded with a revoke-after
// timestamp) -> rebuild client. The old key is revoked by FinishRotations /
// Sweep once the grace elapses; two keys coexist during the overlap.
func (m *Manager) rotateIfDueLocked(ctx context.Context, hash string, d *Delegate) (*Delegate, error) {
	if d.KeyCreated.IsZero() || time.Since(d.KeyCreated) < m.rotateAfter() {
		return d, nil
	}
	logger := log.FromContext(ctx)
	logger.Info("delegate key past rotation cycle, minting replacement",
		"scopeHash", hash, "userId", d.UserID, "keyId", d.KeyID, "keyAge", time.Since(d.KeyCreated).String())

	newKeyID, newKeyJSON, err := m.addKey(withOrg(ctx, d.OrgID), d.UserID)
	if err != nil {
		return nil, fmt.Errorf("rotating delegate key: %w", err)
	}

	// Swap in the Secret first (crash-safe: the new key is persisted before
	// anything depends on it).
	var secret corev1.Secret
	if err := m.K8s.Get(ctx, apitypes.NamespacedName{Name: SecretNamePrefix + hash, Namespace: m.Namespace}, &secret); err != nil {
		return nil, fmt.Errorf("reading delegation Secret for rotation: %w", err)
	}
	now := time.Now().UTC()
	secret.Data[keyDataKey] = newKeyJSON
	secret.Data[keyIDDataKey] = []byte(newKeyID)
	secret.Data[keyCreatedDataKey] = []byte(now.Format(time.RFC3339))
	if d.KeyID != "" {
		secret.Data[oldKeyIDDataKey] = []byte(d.KeyID)
		secret.Data[oldKeyRevokeAtDataKey] = []byte(now.Add(m.rotationGrace()).Format(time.RFC3339))
	}
	if err := m.K8s.Update(ctx, &secret); err != nil {
		return nil, fmt.Errorf("persisting rotated delegation Secret: %w", err)
	}

	nd, err := m.buildDelegate(ctx, newKeyJSON, d.UserID, d.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("building delegated client after rotation: %w", err)
	}
	nd.KeyID = newKeyID
	nd.KeyCreated = now
	nd.validated = true
	m.cache[hash] = nd
	logger.Info("delegate key rotated; old key revokes after grace",
		"scopeHash", hash, "oldKeyId", d.KeyID, "newKeyId", newKeyID, "grace", m.rotationGrace().String())
	return nd, nil
}

// WarmFromSecrets pre-loads cache entries from labeled Secrets (restart path).
func (m *Manager) WarmFromSecrets(ctx context.Context) error {
	var secrets corev1.SecretList
	if err := m.K8s.List(ctx, &secrets,
		client.InNamespace(m.Namespace),
		client.MatchingLabels{DelegationLabel: "true"},
	); err != nil {
		return fmt.Errorf("listing delegation Secrets: %w", err)
	}
	logger := log.FromContext(ctx)
	for i := range secrets.Items {
		s := &secrets.Items[i]
		hash := strings.TrimPrefix(s.Name, SecretNamePrefix)
		if hash == s.Name || len(s.Data[keyDataKey]) == 0 {
			continue
		}
		d, err := m.delegateFromSecret(ctx, s)
		if err != nil {
			logger.Error(err, "warming delegate from Secret failed", "secret", s.Name)
			continue
		}
		m.mu.Lock()
		if m.cache == nil {
			m.cache = map[string]*Delegate{}
		}
		m.cache[hash] = d
		m.mu.Unlock()
		logger.Info("delegate warmed", "secret", s.Name, "userId", d.UserID)
	}
	return nil
}

// Revoke removes the delegate for a scope hash: the Zitadel machine user is
// deleted (revoking all its keys), then the Secret, then the cache entry.
func (m *Manager) Revoke(ctx context.Context, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.revokeLocked(ctx, hash)
}

func (m *Manager) revokeLocked(ctx context.Context, hash string) error {
	logger := log.FromContext(ctx)

	userID, orgID := "", ""
	if d, ok := m.cache[hash]; ok {
		userID, orgID = d.UserID, d.OrgID
	}
	var secret corev1.Secret
	err := m.K8s.Get(ctx, apitypes.NamespacedName{Name: SecretNamePrefix + hash, Namespace: m.Namespace}, &secret)
	secretFound := err == nil
	if err != nil && client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("reading delegation Secret for revoke: %w", err)
	}
	if secretFound && userID == "" {
		userID = string(secret.Data[userIDDataKey])
	}
	if secretFound && orgID == "" {
		orgID = string(secret.Data[orgIDDataKey])
	}

	if userID != "" {
		_, err := m.Binding.Management().RemoveUser(withOrg(ctx, orgID), &management.RemoveUserRequest{Id: userID}) //nolint:staticcheck // SA1019: v1 Management API
		if err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("revoking delegate %s: %w", userID, err)
		}
	}
	if secretFound {
		err := m.K8s.Delete(ctx, &secret)
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("deleting delegation Secret %s: %w", secret.Name, err)
		}
	}
	delete(m.cache, hash)
	logger.Info("delegate revoked", "scopeHash", hash, "userId", userID)
	return nil
}

// Sweep revokes delegates whose scope hash is not in the live set (eager
// revoke on unmatch + periodic orphan GC) and finishes pending key rotations
// whose grace elapsed. The live set is computed by the caller from current
// scope-map resolution across all namespaces.
func (m *Manager) Sweep(ctx context.Context, live map[string]bool) error {
	var secrets corev1.SecretList
	if err := m.K8s.List(ctx, &secrets,
		client.InNamespace(m.Namespace),
		client.MatchingLabels{DelegationLabel: "true"},
	); err != nil {
		return fmt.Errorf("listing delegation Secrets for sweep: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for i := range secrets.Items {
		s := &secrets.Items[i]
		hash := strings.TrimPrefix(s.Name, SecretNamePrefix)
		if hash == s.Name {
			continue
		}
		if !live[hash] {
			if err := m.revokeLocked(ctx, hash); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := m.finishRotationLocked(ctx, s); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// finishRotationLocked revokes the old key of a completed rotation once the
// grace period elapsed.
func (m *Manager) finishRotationLocked(ctx context.Context, secret *corev1.Secret) error {
	oldKeyID := string(secret.Data[oldKeyIDDataKey])
	revokeAtRaw := string(secret.Data[oldKeyRevokeAtDataKey])
	if oldKeyID == "" || revokeAtRaw == "" {
		return nil
	}
	revokeAt, err := time.Parse(time.RFC3339, revokeAtRaw)
	if err != nil || time.Now().Before(revokeAt) {
		return nil //nolint:nilerr // unparsable timestamp: leave for manual cleanup rather than premature revoke
	}
	userID := string(secret.Data[userIDDataKey])
	_, err = m.Binding.Management().RemoveMachineKey(withOrg(ctx, string(secret.Data[orgIDDataKey])), &management.RemoveMachineKeyRequest{ //nolint:staticcheck // SA1019: v1 Management API
		UserId: userID,
		KeyId:  oldKeyID,
	})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("revoking rotated-out key %s of delegate %s: %w", oldKeyID, userID, err)
	}
	delete(secret.Data, oldKeyIDDataKey)
	delete(secret.Data, oldKeyRevokeAtDataKey)
	if err := m.K8s.Update(ctx, secret); err != nil {
		return fmt.Errorf("clearing rotation markers on Secret %s: %w", secret.Name, err)
	}
	log.FromContext(ctx).Info("rotation completed: old delegate key revoked",
		"secret", secret.Name, "oldKeyId", oldKeyID, "userId", userID)
	return nil
}

// mint creates the machine user, key, project (if needed) and membership
// grant for the scope. All grants are explicit; ORG_PROJECT_CREATOR
// self-grant is never relied upon.
func (m *Manager) mint(ctx context.Context, scope *scopemap.Scope, hash string) (*Delegate, []byte, error) {
	orgCtx := metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", scope.OrganizationID)
	username := m.usernamePrefix() + hash

	userID, err := m.ensureMachineUser(orgCtx, username, scope)
	if err != nil {
		return nil, nil, err
	}

	// Resolve/create the project first so scope failures leave no
	// half-granted identity.
	projectID := scope.ProjectID
	if projectID == "" && scope.ProjectName != "" {
		projectID, err = m.ensureProject(orgCtx, scope)
		if err != nil {
			return nil, nil, err
		}
	}

	// Explicit membership grant.
	if projectID == "" {
		if err := m.grantOrgOwner(orgCtx, userID); err != nil {
			return nil, nil, err
		}
	} else {
		if err := m.grantProjectOwner(orgCtx, userID, projectID); err != nil {
			return nil, nil, err
		}
	}

	// Key (created last: a delegate without memberships is useless but
	// harmless; a key without grants would still authenticate).
	keyID, keyJSON, err := m.addKey(orgCtx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("creating machine key for delegate %s: %w", username, err)
	}

	d, err := m.buildDelegate(ctx, keyJSON, userID, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("building delegated client: %w", err)
	}
	d.OrgID = scope.OrganizationID
	d.KeyID = keyID
	d.KeyCreated = time.Now().UTC()
	d.validated = true
	log.FromContext(ctx).Info("delegate minted",
		"scopeHash", hash, "userId", userID, "orgId", scope.OrganizationID,
		"projectId", projectID, "orgScope", projectID == "")
	return d, keyJSON, nil
}

func (m *Manager) addKey(ctx context.Context, userID string) (keyID string, keyJSON []byte, err error) {
	keyResp, err := m.Binding.Management().AddMachineKey(ctx, &management.AddMachineKeyRequest{ //nolint:staticcheck // SA1019: v1 Management API; v2 alternative is UserService.AddKey
		UserId:         userID,
		Type:           authn.KeyType_KEY_TYPE_JSON,
		ExpirationDate: timestamppb.New(time.Now().Add(keyExpiry)),
	})
	if err != nil {
		return "", nil, err
	}
	return keyResp.GetKeyId(), keyResp.GetKeyDetails(), nil
}

func (m *Manager) ensureMachineUser(orgCtx context.Context, username string, scope *scopemap.Scope) (string, error) {
	listResp, err := m.Binding.Management().ListUsers(orgCtx, &management.ListUsersRequest{ //nolint:staticcheck // SA1019: v1 Management API
		Queries: []*userv1.SearchQuery{{
			Query: &userv1.SearchQuery_UserNameQuery{
				UserNameQuery: &userv1.UserNameQuery{
					UserName: username,
					Method:   objectv1.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
				},
			},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("listing users for delegate %s: %w", username, err)
	}
	for _, u := range listResp.GetResult() {
		if u.GetUserName() == username {
			return u.GetId(), nil
		}
	}

	createResp, err := m.Binding.Management().AddMachineUser(orgCtx, &management.AddMachineUserRequest{ //nolint:staticcheck // SA1019: v1 Management API
		UserName:        username,
		Name:            "zitadel-operator delegate (" + scope.MapName + "/" + scope.RuleName + ")",
		Description:     "scope: " + string(scope.KeyJSON()),
		AccessTokenType: userv1.AccessTokenType_ACCESS_TOKEN_TYPE_BEARER,
	})
	if err != nil {
		return "", fmt.Errorf("creating delegate machine user %s: %w", username, err)
	}
	return createResp.GetUserId(), nil
}

func (m *Manager) ensureProject(orgCtx context.Context, scope *scopemap.Scope) (string, error) {
	listResp, err := m.Binding.Project().ListProjects(orgCtx, &projectv2.ListProjectsRequest{
		Filters: []*projectv2.ProjectSearchFilter{{
			Filter: &projectv2.ProjectSearchFilter_ProjectNameFilter{
				ProjectNameFilter: &projectv2.ProjectNameFilter{
					ProjectName: scope.ProjectName,
					Method:      filterv2.TextFilterMethod_TEXT_FILTER_METHOD_EQUALS,
				},
			},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("listing projects for scope project %q: %w", scope.ProjectName, err)
	}
	for _, p := range listResp.GetProjects() {
		if p.GetName() == scope.ProjectName && p.GetOrganizationId() == scope.OrganizationID {
			return p.GetProjectId(), nil
		}
	}

	createResp, err := m.Binding.Project().CreateProject(orgCtx, &projectv2.CreateProjectRequest{
		Name:           scope.ProjectName,
		OrganizationId: scope.OrganizationID,
	})
	if err != nil {
		return "", fmt.Errorf("creating scope project %q: %w", scope.ProjectName, err)
	}
	return createResp.GetProjectId(), nil
}

func (m *Manager) grantOrgOwner(orgCtx context.Context, userID string) error {
	_, err := m.Binding.Management().AddOrgMember(orgCtx, &management.AddOrgMemberRequest{ //nolint:staticcheck // SA1019: v1 Management API
		UserId: userID,
		Roles:  []string{"ORG_OWNER"},
	})
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return fmt.Errorf("granting ORG_OWNER to delegate %s: %w", userID, err)
	}
	return nil
}

func (m *Manager) grantProjectOwner(orgCtx context.Context, userID, projectID string) error {
	_, err := m.Binding.Management().AddProjectMember(orgCtx, &management.AddProjectMemberRequest{ //nolint:staticcheck // SA1019: v1 Management API
		ProjectId: projectID,
		UserId:    userID,
		Roles:     []string{"PROJECT_OWNER"},
	})
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return fmt.Errorf("granting PROJECT_OWNER on %s to delegate %s: %w", projectID, userID, err)
	}
	return nil
}

func (m *Manager) buildDelegate(ctx context.Context, keyJSON []byte, userID, projectID string) (*Delegate, error) {
	cfg := m.ClientCfg
	cfg.KeyJSON = keyJSON
	c, err := zitadel.NewClient(ctx, &cfg)
	if err != nil {
		return nil, err
	}
	return &Delegate{Client: c, UserID: userID, ProjectID: projectID}, nil
}

func (m *Manager) delegateFromSecret(ctx context.Context, secret *corev1.Secret) (*Delegate, error) {
	d, err := m.buildDelegate(ctx, secret.Data[keyDataKey],
		string(secret.Data[userIDDataKey]), string(secret.Data[projectIDDataKey]))
	if err != nil {
		return nil, err
	}
	d.OrgID = string(secret.Data[orgIDDataKey])
	d.KeyID = string(secret.Data[keyIDDataKey])
	if raw := string(secret.Data[keyCreatedDataKey]); raw != "" {
		if t, perr := time.Parse(time.RFC3339, raw); perr == nil {
			d.KeyCreated = t
		}
	}
	return d, nil
}

func (m *Manager) persistSecret(ctx context.Context, scope *scopemap.Scope, hash string, keyJSON []byte, d *Delegate) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretNamePrefix + hash,
			Namespace: m.Namespace,
			Labels:    map[string]string{DelegationLabel: "true"},
		},
		Data: map[string][]byte{
			keyDataKey:        keyJSON,
			scopeDataKey:      scope.KeyJSON(),
			userIDDataKey:     []byte(d.UserID),
			orgIDDataKey:      []byte(scope.OrganizationID),
			projectIDDataKey:  []byte(d.ProjectID),
			keyIDDataKey:      []byte(d.KeyID),
			keyCreatedDataKey: []byte(d.KeyCreated.Format(time.RFC3339)),
		},
	}
	return m.K8s.Create(ctx, secret)
}
