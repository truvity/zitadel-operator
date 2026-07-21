package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/idp"
)

// GoogleIdPReconciler reconciles a GoogleIdP object.
type GoogleIdPReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=googleidps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=googleidps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=googleidps/finalizers,verbs=update

func (r *GoogleIdPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.GoogleIdP
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.IdpID != "" {
			_, err := r.Zitadel.Admin().RemoveIDPFromLoginPolicy(ctx, &admin.RemoveIDPFromLoginPolicyRequest{
				IdpId: cr.Status.IdpID,
			})
			// Ignore not found — might not be in login policy.
			if err != nil && status.Code(err) != codes.NotFound {
				logger.Info("could not remove idp from login policy", "error", err)
			}
			// TODO: Admin API doesn't have a direct "delete provider" for instance-level IdPs.
			// The provider is deactivated by removing it from the login policy.
		}
		if removeFinalizer(&cr) {
			if err := r.Update(ctx, &cr); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer.
	if addFinalizer(&cr) {
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Resolve client secret from Secret.
	clientSecret, err := r.resolveClientSecret(ctx, &cr)
	if err != nil {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "SecretNotFound", err.Error())
		_ = applyStatus(ctx, r.Client, r.Config, &cr)
		return ctrl.Result{RequeueAfter: requeueOnError}, nil
	}

	// Ensure Google IdP exists.
	idpID, err := r.ensureGoogleIdP(ctx, &cr, clientSecret)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	if cr.Status.IdpID != idpID || !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.IdpID = idpID
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		if err := applyStatus(ctx, r.Client, r.Config, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("googleidp reconciled", "idpID", idpID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *GoogleIdPReconciler) resolveClientSecret(ctx context.Context, cr *zitadelv1alpha2.GoogleIdP) (string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cr.Spec.ClientSecretRef.Name,
		Namespace: cr.Namespace,
	}, secret); err != nil {
		return "", fmt.Errorf("getting clientSecretRef secret %s: %w", cr.Spec.ClientSecretRef.Name, err)
	}

	key := cr.Spec.ClientSecretRef.Key
	if key == "" {
		key = "clientSecret"
	}

	data, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s", key, cr.Spec.ClientSecretRef.Name)
	}

	return string(data), nil
}

func (r *GoogleIdPReconciler) ensureGoogleIdP(ctx context.Context, cr *zitadelv1alpha2.GoogleIdP, clientSecret string) (string, error) {
	providerOptions := &idp.Options{
		IsLinkingAllowed:  cr.Spec.IsLinkingAllowed,
		IsCreationAllowed: cr.Spec.IsCreationAllowed,
		IsAutoCreation:    cr.Spec.IsAutoCreation,
		IsAutoUpdate:      cr.Spec.IsAutoUpdate,
	}

	// If we already have an IdP ID, update it.
	if cr.Status.IdpID != "" {
		_, err := r.Zitadel.Admin().UpdateGoogleProvider(ctx, &admin.UpdateGoogleProviderRequest{
			Id:              cr.Status.IdpID,
			Name:            cr.DisplayName(),
			ClientId:        cr.Spec.ClientId,
			ClientSecret:    clientSecret,
			Scopes:          cr.Spec.Scopes,
			ProviderOptions: providerOptions,
		})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// IdP deleted externally, recreate below.
			} else {
				return "", fmt.Errorf("updating google provider: %w", err)
			}
		} else {
			return cr.Status.IdpID, nil
		}
	}

	// Create new Google IdP.
	resp, err := r.Zitadel.Admin().AddGoogleProvider(ctx, &admin.AddGoogleProviderRequest{
		Name:            cr.DisplayName(),
		ClientId:        cr.Spec.ClientId,
		ClientSecret:    clientSecret,
		Scopes:          cr.Spec.Scopes,
		ProviderOptions: providerOptions,
	})
	if err != nil {
		return "", fmt.Errorf("adding google provider: %w", err)
	}

	return resp.GetId(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GoogleIdPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.GoogleIdP{}).
		Named("googleidp").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
