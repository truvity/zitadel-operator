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
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// PersonalAccessTokenReconciler reconciles a PersonalAccessToken object.
type PersonalAccessTokenReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
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

	// Resolve organization.
	orgID, err := resolveOrganizationId(ctx, r.Client, r.Config, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for organization ref to become ready", "error", err)
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "OrgNotReady", err.Error())
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Resolve user ID.
	userID, err := resolveUserId(ctx, r.Client, cr.Spec.UserRef, cr.Spec.UserId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for user ref to become ready", "error", err)
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "UserNotReady", err.Error())
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving user: %w", err)
	}

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		if cr.Status.TokenId != "" {
			_, err := r.Zitadel.Management().RemovePersonalAccessToken(ctx, &management.RemovePersonalAccessTokenRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
				UserId:  userID,
				TokenId: cr.Status.TokenId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return fmt.Errorf("removing personal access token: %w", err)
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

	// Ensure token exists and is stored in Secret.
	if err := r.ensureToken(ctx, &cr, userID); err != nil {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "TokenError", err.Error())
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := cr.Status.UserId != userID || cr.Status.OrganizationId != orgID
	cr.Status.UserId = userID
	cr.Status.OrganizationId = orgID
	if err := markReady(ctx, r.Client, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("personalaccesstoken reconciled", "tokenId", cr.Status.TokenId, "userId", userID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
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

	// Create personal access token via Management API.
	tokenResp, err := r.Zitadel.Management().AddPersonalAccessToken(ctx, &management.AddPersonalAccessTokenRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
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
