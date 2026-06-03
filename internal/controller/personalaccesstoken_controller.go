package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// PersonalAccessTokenReconciler reconciles a PersonalAccessToken object.
type PersonalAccessTokenReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=personalaccesstokens,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=personalaccesstokens/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=personalaccesstokens/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *PersonalAccessTokenReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha1.PersonalAccessToken
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.TokenId != "" {
			_, err := r.Zitadel.Management().RemovePersonalAccessToken(ctx, &management.RemovePersonalAccessTokenRequest{ //nolint:staticcheck // v1 API required, v2 not yet available in SDK
				UserId:  cr.Spec.UserId,
				TokenId: cr.Status.TokenId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("removing personal access token: %w", err)
			}
		}
		// Delete the token secret.
		if cr.Spec.TokenSecretRef.Name != "" {
			secret := &corev1.Secret{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      cr.Spec.TokenSecretRef.Name,
				Namespace: cr.Namespace,
			}, secret)
			if err == nil {
				if delErr := r.Delete(ctx, secret); delErr != nil {
					logger.Error(delErr, "failed to delete token secret")
				}
			}
		}
		if removeFinalizer(&cr) {
			if err := r.Update(ctx, &cr); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present.
	if addFinalizer(&cr) {
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// If we already have a token ID, the token is created (tokens are immutable).
	if cr.Status.TokenId != "" {
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// Compute expiration date.
	expirationDays := 365
	if cr.Spec.ExpirationDays > 0 {
		expirationDays = cr.Spec.ExpirationDays
	}
	expirationDate := timestamppb.New(time.Now().AddDate(0, 0, expirationDays))

	// Create personal access token.
	resp, err := r.Zitadel.Management().AddPersonalAccessToken(ctx, &management.AddPersonalAccessTokenRequest{ //nolint:staticcheck // v1 API required, v2 not yet available in SDK
		UserId:         cr.Spec.UserId,
		ExpirationDate: expirationDate,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("adding personal access token: %w", err)
	}

	tokenID := resp.GetTokenId()
	token := resp.GetToken()

	// Write token to K8s Secret.
	if err := r.ensureTokenSecret(ctx, &cr, token); err != nil {
		return ctrl.Result{}, fmt.Errorf("writing token secret: %w", err)
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.TokenId = tokenID
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	cr.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Synced",
			Message:            fmt.Sprintf("PersonalAccessToken created for user %q", cr.Spec.UserId),
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("personalaccesstoken reconciled", "tokenId", tokenID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *PersonalAccessTokenReconciler) ensureTokenSecret(ctx context.Context, cr *zitadelv1alpha1.PersonalAccessToken, token string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.TokenSecretRef.Name,
			Namespace: cr.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		// Use "token" as the key, or configurable via Keys.ClientId (repurpose).
		tokenKey := "token"
		if cr.Spec.TokenSecretRef.Keys != nil && cr.Spec.TokenSecretRef.Keys.ClientId != "" {
			tokenKey = cr.Spec.TokenSecretRef.Keys.ClientId
		}
		secret.Data[tokenKey] = []byte(token)
		// Write extra data entries into the Secret.
		for k, v := range cr.Spec.TokenSecretRef.ExtraData {
			secret.Data[k] = []byte(v)
		}
		return nil
	})
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *PersonalAccessTokenReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.PersonalAccessToken{}).
		Named("personalaccesstoken").
		Complete(r)
}
