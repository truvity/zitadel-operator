package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/authn"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	objectv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object"
	userv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user"
)

// MachineUserReconciler reconciles a MachineUser object.
type MachineUserReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager

	// instanceIDOnce caches the best-effort instance ID lookup (INF-426
	// connection bundle).
	instanceIDOnce   sync.Once
	cachedInstanceID string
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=machineusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=machineusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=machineusers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func (r *MachineUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.MachineUser
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// v0.18 (INF-422/INF-423): dual-serving instance gate + scope
	// resolution. Fail-closed outcomes return immediately; during deletion
	// failures fall back to the binding client so finalizers cannot deadlock.
	ctx, rs, rsDone, rsResult, rsErr := tenantPreamble(ctx, r.Client, r.Config,
		r.Resolver, r.Delegation, r.Zitadel, &cr, cr.Spec.Instance, &cr.Status.Conditions, req.Namespace)
	if rsDone {
		return rsResult, rsErr
	}

	// Resolve organization.
	orgID, err := resolveScopedOrganizationId(ctx, r.Client, rs, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "OrgNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for Management API calls.
	ctx = withOrgID(ctx, orgID)

	// Identity ops (user create/delete, keys) need org-level user
	// permissions. An org-scope delegate (ORG_OWNER) has them; a
	// project-scope delegate (PROJECT_OWNER) does not — identity minting
	// stays a binding-level operation (the same trust boundary as delegation
	// minting), while role grants go through the delegated client.
	userCtx := r.identityOpsContext(ctx, rs)

	// Deletion.
	if done, result, err := handleDeletion(userCtx, r.Client, &cr, func() error {
		if cr.Status.UserId != "" {
			_, err := zclient(userCtx, r.Zitadel).Management().RemoveUser(userCtx, &management.RemoveUserRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
				Id: cr.Status.UserId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return fmt.Errorf("deleting machine user: %w", err)
			}
		}
		return nil
	}); done {
		return result, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}
	// ensureFinalizer's full-object Update refreshed the object from the
	// server, dropping in-memory condition edits — re-apply ScopeResolved.
	applyScopeResolvedCondition(rs, &cr.Status.Conditions)

	// Ensure machine user exists.
	userID, err := r.ensureMachineUser(userCtx, &cr, orgID)
	if err != nil {
		return ctrl.Result{}, err
	}
	prevStatus := cr.Status

	// v0.18 (INF-426): project role grants within the resolved scope.
	if done, result, err := r.ensureRoleGrant(ctx, &cr, rs, userID); done {
		return result, err
	}

	// Ensure key exists in the connection-bundle Secret (dual-key rotation
	// when spec.key.rotateAfter is set).
	if err := r.ensureKey(userCtx, &cr, userID, orgID); err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	cr.Status.UserId = userID
	cr.Status.OrganizationId = orgID
	statusChanged := machineUserStatusChanged(&prevStatus, &cr.Status)
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("machineuser reconciled", "userId", userID, "orgId", orgID,
		"keyId", cr.Status.KeyId, "grantId", cr.Status.GrantId)
	return ctrl.Result{RequeueAfter: r.nextRequeue(&cr)}, nil
}

// identityOpsContext returns the context whose client performs machine-user
// identity operations: the delegated client for org scopes, the binding
// client for project scopes (PROJECT_OWNER cannot manage org users).
func (r *MachineUserReconciler) identityOpsContext(ctx context.Context, rs resolvedScope) context.Context {
	if rs.scope != nil && rs.delegate != nil && rs.delegate.ProjectID != "" {
		return withZClient(ctx, r.Zitadel)
	}
	return ctx
}

// machineUserStatusChanged reports whether any persisted status field moved
// (INF-430 class: every ID the deletion path revokes must be persisted).
func machineUserStatusChanged(prev, cur *zitadelv1alpha2.MachineUserStatus) bool {
	return cur.UserId != prev.UserId ||
		cur.OrganizationId != prev.OrganizationId ||
		cur.ProjectId != prev.ProjectId ||
		cur.GrantId != prev.GrantId ||
		cur.KeyId != prev.KeyId ||
		cur.PreviousKeyId != prev.PreviousKeyId
}

// nextRequeue shortens the requeue while a rotated-out key awaits revocation.
func (r *MachineUserReconciler) nextRequeue(cr *zitadelv1alpha2.MachineUser) time.Duration {
	if cr.Status.PreviousKeyRevokeAt != nil {
		if until := time.Until(cr.Status.PreviousKeyRevokeAt.Time) + time.Second; until < requeueInterval {
			if until < time.Second {
				return time.Second
			}
			return until
		}
	}
	return requeueInterval
}

func (r *MachineUserReconciler) ensureMachineUser(ctx context.Context, cr *zitadelv1alpha2.MachineUser, _ string) (string, error) {
	// If we already have a user ID, verify it still exists.
	if cr.Status.UserId != "" {
		_, err := zclient(ctx, r.Zitadel).Management().GetUserByID(ctx, &management.GetUserByIDRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
			Id: cr.Status.UserId,
		})
		if err == nil {
			return cr.Status.UserId, nil
		}
		if status.Code(err) != codes.NotFound {
			return "", fmt.Errorf("getting user by ID: %w", err)
		}
		// User was deleted externally, recreate.
	}

	// Search by username via Management API.
	listResp, err := zclient(ctx, r.Zitadel).Management().ListUsers(ctx, &management.ListUsersRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		Queries: []*userv1.SearchQuery{
			{
				Query: &userv1.SearchQuery_UserNameQuery{
					UserNameQuery: &userv1.UserNameQuery{
						UserName: cr.Spec.UserName,
						Method:   objectv1.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("listing users: %w", err)
	}

	for _, u := range listResp.GetResult() {
		if u.GetUserName() == cr.Spec.UserName {
			return u.GetId(), nil
		}
	}

	// Resolve access token type.
	accessTokenType := userv1.AccessTokenType_ACCESS_TOKEN_TYPE_BEARER
	if cr.Spec.AccessTokenType == "jwt" {
		accessTokenType = userv1.AccessTokenType_ACCESS_TOKEN_TYPE_JWT
	}

	// Create machine user via Management API.
	createResp, err := zclient(ctx, r.Zitadel).Management().AddMachineUser(ctx, &management.AddMachineUserRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		UserName:        cr.Spec.UserName,
		Name:            cr.DisplayName(),
		Description:     cr.Spec.Description,
		AccessTokenType: accessTokenType,
	})
	if err != nil {
		return "", fmt.Errorf("creating machine user: %w", err)
	}

	return createResp.GetUserId(), nil
}

// ensureKey guarantees a machine key exists in the referenced Secret and,
// when spec.key.rotateAfter is set, drives dual-key rotation: mint new key ->
// swap into the Secret -> revoke the old key after grace (two keys coexist
// during the overlap). The Secret is a full connection bundle (INF-426):
// key.json + instanceUrl + issuer (+ instanceId when resolvable) + orgId
// (+ projectId for project scopes) — a consumer can construct a working
// client from the Secret alone. Backward compatible: no rotateAfter = keys
// are never rotated, existing Secret data keys keep their names.
func (r *MachineUserReconciler) ensureKey(ctx context.Context, cr *zitadelv1alpha2.MachineUser, userID, orgID string) error {
	secretKey := cr.Spec.KeySecretRef.Key
	if secretKey == "" {
		secretKey = "key.json"
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: cr.Spec.KeySecretRef.Name, Namespace: cr.Namespace}, secret)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("reading key secret: %w", err)
	}
	hasKey := err == nil && len(secret.Data[secretKey]) > 0

	// Finish a pending rotation whose grace elapsed.
	if err := r.finishPendingKeyRevocation(ctx, cr, userID); err != nil {
		return err
	}

	rotationDue := keyRotationDue(cr, hasKey)

	if hasKey && !rotationDue {
		// Ensure the bundle fields are present even for pre-existing Secrets.
		return r.ensureBundle(ctx, cr, secret, orgID)
	}

	// Mint a new key (first key or rotation replacement).
	expiration := time.Now().Add(365 * 10 * 24 * time.Hour)                                                   // 10 years backstop
	keyResp, err := zclient(ctx, r.Zitadel).Management().AddMachineKey(ctx, &management.AddMachineKeyRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		UserId:         userID,
		Type:           authn.KeyType_KEY_TYPE_JSON,
		ExpirationDate: timestamppb.New(expiration),
	})
	if err != nil {
		return fmt.Errorf("creating machine key: %w", err)
	}
	keyJSON := keyResp.GetKeyDetails()
	now := metav1.Now()

	if rotationDue {
		recordKeyRotation(ctx, cr, keyResp.GetKeyId(), now)
	}
	cr.Status.KeyId = keyResp.GetKeyId()
	cr.Status.KeyCreatedAt = &now

	return r.writeKeyBundleSecret(ctx, cr, secret, secretKey, keyJSON, orgID)
}

// writeKeyBundleSecret creates or updates the connection-bundle Secret with
// the (possibly rotated) key JSON.
func (r *MachineUserReconciler) writeKeyBundleSecret(ctx context.Context, cr *zitadelv1alpha2.MachineUser, secret *corev1.Secret, secretKey string, keyJSON []byte, orgID string) error {
	if secret.Name == "" {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Spec.KeySecretRef.Name,
				Namespace: cr.Namespace,
			},
			Data: map[string][]byte{secretKey: keyJSON},
		}
		r.setBundleData(ctx, cr, secret, orgID)
		return r.Create(ctx, secret)
	}
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[secretKey] = keyJSON
	r.setBundleData(ctx, cr, secret, orgID)
	return r.Update(ctx, secret)
}

// finishPendingKeyRevocation revokes the rotated-out key once its grace
// elapsed and clears the rotation bookkeeping.
func (r *MachineUserReconciler) finishPendingKeyRevocation(ctx context.Context, cr *zitadelv1alpha2.MachineUser, userID string) error {
	if cr.Status.PreviousKeyId == "" || cr.Status.PreviousKeyRevokeAt == nil ||
		time.Now().Before(cr.Status.PreviousKeyRevokeAt.Time) {
		return nil
	}
	_, rmErr := zclient(ctx, r.Zitadel).Management().RemoveMachineKey(ctx, &management.RemoveMachineKeyRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method
		UserId: userID,
		KeyId:  cr.Status.PreviousKeyId,
	})
	if rmErr != nil && status.Code(rmErr) != codes.NotFound {
		return fmt.Errorf("revoking rotated-out key %s: %w", cr.Status.PreviousKeyId, rmErr)
	}
	log.FromContext(ctx).Info("rotation completed: old machine key revoked", "keyId", cr.Status.PreviousKeyId)
	cr.Status.PreviousKeyId = ""
	cr.Status.PreviousKeyRevokeAt = nil
	return nil
}

// DefaultKeyRotationGrace is the default old-key validity after rotation.
const DefaultKeyRotationGrace = 5 * time.Minute

// keyRotationDue reports whether the current key must be replaced. A key of
// unknown age (created before v0.18 bookkeeping) counts as due — rotation
// converges it into tracked state.
func keyRotationDue(cr *zitadelv1alpha2.MachineUser, hasKey bool) bool {
	if !hasKey || cr.Spec.Key == nil || cr.Spec.Key.RotateAfter == nil || cr.Status.PreviousKeyId != "" {
		return false
	}
	return cr.Status.KeyCreatedAt == nil ||
		time.Since(cr.Status.KeyCreatedAt.Time) >= cr.Spec.Key.RotateAfter.Duration
}

// recordKeyRotation stamps the dual-key rotation bookkeeping: the outgoing
// key is kept valid until the grace elapses.
func recordKeyRotation(ctx context.Context, cr *zitadelv1alpha2.MachineUser, newKeyID string, now metav1.Time) {
	grace := DefaultKeyRotationGrace
	if cr.Spec.Key.RotationGrace != nil {
		grace = cr.Spec.Key.RotationGrace.Duration
	}
	revokeAt := metav1.NewTime(now.Add(grace))
	cr.Status.PreviousKeyId = cr.Status.KeyId // empty for legacy untracked keys
	cr.Status.PreviousKeyRevokeAt = &revokeAt
	log.FromContext(ctx).Info("machine key rotated; old key revokes after grace",
		"oldKeyId", cr.Status.KeyId, "newKeyId", newKeyID, "grace", grace.String())
}

// ensureBundle adds any missing connection-bundle fields to an existing Secret.
func (r *MachineUserReconciler) ensureBundle(ctx context.Context, cr *zitadelv1alpha2.MachineUser, secret *corev1.Secret, orgID string) error {
	before := len(secret.Data)
	changed := false
	for k, v := range r.bundleFields(ctx, cr, orgID) {
		if string(secret.Data[k]) != v {
			secret.Data[k] = []byte(v)
			changed = true
		}
	}
	if changed || len(secret.Data) != before {
		return r.Update(ctx, secret)
	}
	return nil
}

func (r *MachineUserReconciler) setBundleData(ctx context.Context, cr *zitadelv1alpha2.MachineUser, secret *corev1.Secret, orgID string) {
	for k, v := range r.bundleFields(ctx, cr, orgID) {
		secret.Data[k] = []byte(v)
	}
}

// bundleFields computes the non-key connection-bundle entries.
func (r *MachineUserReconciler) bundleFields(ctx context.Context, cr *zitadelv1alpha2.MachineUser, orgID string) map[string]string {
	scheme := "https"
	if r.Config != nil && r.Config.Insecure {
		scheme = "http"
	}
	domain, port := "", "443"
	if r.Config != nil {
		domain, port = r.Config.Domain, r.Config.Port
	}
	instanceURL := scheme + "://" + domain
	if port != "" && port != "443" {
		instanceURL += ":" + port
	}
	issuerDomain := domain
	if r.Config != nil && r.Config.ExternalDomain != "" {
		issuerDomain = r.Config.ExternalDomain
	}
	fields := map[string]string{
		"instanceUrl": instanceURL,
		"issuer":      "https://" + issuerDomain,
		"orgId":       orgID,
	}
	if cr.Status.ProjectId != "" {
		fields["projectId"] = cr.Status.ProjectId
	}
	if id := r.instanceID(ctx); id != "" {
		fields["instanceId"] = id
	}
	return fields
}

// instanceID resolves the Zitadel instance ID once (best-effort: requires an
// iam-owner binding; org-owner bindings simply omit the field).
func (r *MachineUserReconciler) instanceID(ctx context.Context) string {
	r.instanceIDOnce.Do(func() {
		resp, err := r.Zitadel.Admin().GetMyInstance(ctx, &admin.GetMyInstanceRequest{}) //nolint:staticcheck // SA1019: v1 Admin API; v2 instance service is a separate migration
		if err != nil {
			log.FromContext(ctx).Info("instance ID lookup failed; connection bundles omit instanceId", "error", err)
			return
		}
		r.cachedInstanceID = resp.GetInstance().GetId()
	})
	return r.cachedInstanceID
}

// withOrgID adds x-zitadel-orgid metadata to the context for Management API calls.
func withOrgID(ctx context.Context, orgID string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MachineUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.MachineUser{}).
		Named("machineuser").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
