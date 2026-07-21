package controller

import (
	"context"
	"fmt"
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

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// PersonalAccessTokenReconciler reconciles a PersonalAccessToken object.
type PersonalAccessTokenReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=personalaccesstokens,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=personalaccesstokens/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=personalaccesstokens/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func (r *PersonalAccessTokenReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.PersonalAccessToken
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

	// Resolve user ID.
	userID, err := resolveUserId(ctx, r.Client, cr.Spec.UserRef, cr.Spec.UserId, cr.Namespace)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "UserNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving user: %w", err)
	}

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, r.deleteTokenFunc(ctx, &cr, userID)); done {
		return result, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}
	// ensureFinalizer's full-object Update refreshed the object from the
	// server, dropping in-memory condition edits — re-apply ScopeResolved.
	applyScopeResolvedCondition(rs, &cr.Status.Conditions)

	// Ensure token exists and is stored in Secret.
	prevStatusTokenID := cr.Status.TokenId
	if err := r.ensureToken(ctx, &cr, userID); err != nil {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "TokenError", err.Error())
		_ = applyStatus(ctx, r.Client, r.Config, &cr)
		return ctrl.Result{}, err
	}

	// Status. TokenId must participate in change detection (INF-430 audit):
	// a re-minted token whose ID is never persisted leaves deletion revoking
	// a stale token and leaking the live one.
	statusChanged := cr.Status.UserId != userID || cr.Status.OrganizationId != orgID ||
		cr.Status.TokenId != prevStatusTokenID
	cr.Status.UserId = userID
	cr.Status.OrganizationId = orgID
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("personalaccesstoken reconciled", "tokenId", cr.Status.TokenId, "userId", userID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// deleteTokenFunc returns the cleanup closure for handleDeletion.
func (r *PersonalAccessTokenReconciler) deleteTokenFunc(ctx context.Context, cr *zitadelv1alpha2.PersonalAccessToken, userID string) func() error {
	return func() error {
		if cr.Status.TokenId == "" {
			return nil
		}
		_, err := zclient(ctx, r.Zitadel).Management().RemovePersonalAccessToken(ctx, &management.RemovePersonalAccessTokenRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
			UserId:  userID,
			TokenId: cr.Status.TokenId,
		})
		if err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("removing personal access token: %w", err)
		}
		return nil
	}
}

func (r *PersonalAccessTokenReconciler) ensureToken(ctx context.Context, cr *zitadelv1alpha2.PersonalAccessToken, userID string) error {
	secretKey := cr.Spec.TokenSecretRef.Key
	if secretKey == "" {
		secretKey = "token"
	}

	// Check if Secret already exists with token data.
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: cr.Spec.TokenSecretRef.Name, Namespace: cr.Namespace}, secret)
	if err == nil && len(secret.Data[secretKey]) > 0 && cr.Status.TokenId != "" {
		return nil // Token already stored.
	}

	// Determine expiration.
	expiration := time.Now().Add(365 * 10 * 24 * time.Hour) // 10 years
	if cr.Spec.ExpirationDate != nil {
		expiration = cr.Spec.ExpirationDate.Time
	}

	// Replacing a token whose Secret was lost: revoke the old one first so it
	// does not linger past CR deletion (INF-430 audit).
	if cr.Status.TokenId != "" {
		_, rmErr := zclient(ctx, r.Zitadel).Management().RemovePersonalAccessToken(ctx, &management.RemovePersonalAccessTokenRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method
			UserId:  userID,
			TokenId: cr.Status.TokenId,
		})
		if rmErr != nil && status.Code(rmErr) != codes.NotFound {
			return fmt.Errorf("revoking replaced personal access token: %w", rmErr)
		}
	}

	// Create personal access token via Management API.
	tokenResp, err := zclient(ctx, r.Zitadel).Management().AddPersonalAccessToken(ctx, &management.AddPersonalAccessTokenRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		UserId:         userID,
		ExpirationDate: timestamppb.New(expiration),
	})
	if err != nil {
		return fmt.Errorf("creating personal access token: %w", err)
	}

	cr.Status.TokenId = tokenResp.GetTokenId()
	token := tokenResp.GetToken()

	// Store in Secret.
	if secret.Name == "" {
		// Create new secret.
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Spec.TokenSecretRef.Name,
				Namespace: cr.Namespace,
			},
			Data: map[string][]byte{
				secretKey: []byte(token),
			},
		}
		return r.Create(ctx, newSecret)
	}

	// Update existing secret.
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[secretKey] = []byte(token)
	return r.Update(ctx, secret)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PersonalAccessTokenReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.PersonalAccessToken{}).
		Named("personalaccesstoken").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
