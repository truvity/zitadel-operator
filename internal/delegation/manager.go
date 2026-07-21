// Package delegation mints and caches per-scope Zitadel machine users
// ("delegates") and hands out Zitadel clients authenticated as them.
//
// v0.18 prototype (proto/v018-scope-maps). Per resolved scope:
//   - explicit machine-user create (never relies on ORG_PROJECT_CREATOR
//     self-grant)
//   - explicit membership grant: org-scope => ORG_OWNER org member,
//     project-scope => PROJECT_OWNER member on the project (project created
//     if missing, using the binding credential)
//   - key JSON cached in a labeled Secret zitadel-delegation-<scope-hash>
//     in the operator namespace; restart warms from those Secrets.
package delegation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

	// usernamePrefix keeps prototype resources identifiable and cleanable.
	usernamePrefix = "proto018-delegate-"

	keyDataKey       = "key.json"
	scopeDataKey     = "scope.json"
	userIDDataKey    = "user_id"
	projectIDDataKey = "project_id"
)

// Delegate is a minted per-scope identity plus a client acting as it.
type Delegate struct {
	// Client is a Zitadel client authenticated as the delegated machine user.
	Client *zitadel.Client
	// UserID is the delegated machine user's Zitadel ID.
	UserID string
	// ProjectID is the resolved project ID (empty for org-scope).
	ProjectID string
}

// Manager mints, persists and caches delegates.
type Manager struct {
	// K8s writes delegation Secrets (operator namespace).
	K8s client.Client
	// Binding is the operator's own (high-privilege) Zitadel client,
	// used ONLY to mint delegates, never to reconcile tenant resources
	// once a scope is delegated.
	Binding *zitadel.Client
	// ClientCfg is the connection template for delegated clients
	// (KeyJSON is replaced per delegate).
	ClientCfg zitadel.ClientConfig
	// Namespace is the operator namespace holding delegation Secrets.
	Namespace string

	mu    sync.Mutex
	cache map[string]*Delegate // scope hash -> delegate
}

// Ensure returns a delegate for the scope, minting it if needed.
// Order: in-memory cache -> Secret (warm restart) -> mint against Zitadel.
func (m *Manager) Ensure(ctx context.Context, scope *scopemap.Scope) (*Delegate, error) {
	hash := scope.Hash()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cache == nil {
		m.cache = map[string]*Delegate{}
	}
	if d, ok := m.cache[hash]; ok {
		return d, nil
	}

	// Warm from Secret.
	var secret corev1.Secret
	err := m.K8s.Get(ctx, apitypes.NamespacedName{Name: SecretNamePrefix + hash, Namespace: m.Namespace}, &secret)
	if err == nil && len(secret.Data[keyDataKey]) > 0 {
		d, buildErr := m.buildDelegate(ctx, secret.Data[keyDataKey], string(secret.Data[userIDDataKey]), string(secret.Data[projectIDDataKey]))
		if buildErr != nil {
			return nil, fmt.Errorf("building delegated client from Secret %s: %w", secret.Name, buildErr)
		}
		m.cache[hash] = d
		log.FromContext(ctx).Info("delegation warmed from Secret", "scopeHash", hash, "userId", d.UserID)
		return d, nil
	}
	if err != nil && client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("reading delegation Secret: %w", err)
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
		d, err := m.buildDelegate(ctx, s.Data[keyDataKey], string(s.Data[userIDDataKey]), string(s.Data[projectIDDataKey]))
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

// mint creates the machine user, key, project (if needed) and membership
// grant for the scope. All grants are explicit; ORG_PROJECT_CREATOR
// self-grant is never relied upon.
func (m *Manager) mint(ctx context.Context, scope *scopemap.Scope, hash string) (*Delegate, []byte, error) {
	orgCtx := metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", scope.OrganizationID)
	username := usernamePrefix + hash

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
	keyResp, err := m.Binding.Management().AddMachineKey(orgCtx, &management.AddMachineKeyRequest{ //nolint:staticcheck // SA1019: v1 Management API; v2 alternative is UserService.AddKey
		UserId:         userID,
		Type:           authn.KeyType_KEY_TYPE_JSON,
		ExpirationDate: timestamppb.New(time.Now().Add(365 * 24 * time.Hour)),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating machine key for delegate %s: %w", username, err)
	}
	keyJSON := keyResp.GetKeyDetails()

	d, err := m.buildDelegate(ctx, keyJSON, userID, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("building delegated client: %w", err)
	}
	log.FromContext(ctx).Info("delegate minted",
		"scopeHash", hash, "userId", userID, "orgId", scope.OrganizationID,
		"projectId", projectID, "orgScope", projectID == "")
	return d, keyJSON, nil
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
		Name:            "v0.18 delegated operator SA (" + scope.MapName + "/" + scope.RuleName + ")",
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

func (m *Manager) persistSecret(ctx context.Context, scope *scopemap.Scope, hash string, keyJSON []byte, d *Delegate) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretNamePrefix + hash,
			Namespace: m.Namespace,
			Labels:    map[string]string{DelegationLabel: "true"},
		},
		Data: map[string][]byte{
			keyDataKey:       keyJSON,
			scopeDataKey:     scope.KeyJSON(),
			userIDDataKey:    []byte(d.UserID),
			projectIDDataKey: []byte(d.ProjectID),
		},
	}
	if err := m.K8s.Create(ctx, secret); err != nil {
		return err
	}
	return nil
}
