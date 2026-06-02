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

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	idpv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/idp"
)

// IdentityProviderReconciler reconciles an IdentityProvider object.
type IdentityProviderReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=identityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=identityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=identityproviders/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *IdentityProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the IdentityProvider CR.
	var cr zitadelv1alpha1.IdentityProvider
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.IdpId != "" {
			_, err := r.Zitadel.Admin().RemoveIDP(ctx, &admin.RemoveIDPRequest{
				IdpId: cr.Status.IdpId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("deleting IDP: %w", err)
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

	// Read client credentials from referenced K8s Secrets.
	clientID, err := r.readSecretKey(ctx, cr.Spec.ClientIdFromSecret)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reading client ID secret: %w", err)
	}
	clientSecret, err := r.readSecretKey(ctx, cr.Spec.ClientSecretFromSecret)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reading client secret: %w", err)
	}

	// Create or update IDP.
	idpID, err := r.ensureIDP(ctx, &cr, clientID, clientSecret)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.IdpId = idpID
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("identityprovider reconciled", "idpId", idpID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *IdentityProviderReconciler) readSecretKey(ctx context.Context, ref zitadelv1alpha1.SecretKeyRef) (string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}, secret); err != nil {
		return "", err
	}

	data, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s/%s", ref.Key, ref.Namespace, ref.Name)
	}
	return string(data), nil
}

func (r *IdentityProviderReconciler) ensureIDP(ctx context.Context, cr *zitadelv1alpha1.IdentityProvider, clientID, clientSecret string) (string, error) {
	providerOptions := &idpv1.Options{
		IsCreationAllowed: cr.Spec.AutoCreation,
		IsAutoCreation:    cr.Spec.AutoCreation,
		IsAutoUpdate:      cr.Spec.AutoUpdate,
		IsLinkingAllowed:  true,
	}

	// If we already have an IDP ID, update it.
	if cr.Status.IdpId != "" {
		_, err := r.Zitadel.Admin().UpdateGoogleProvider(ctx, &admin.UpdateGoogleProviderRequest{
			Id:              cr.Status.IdpId,
			Name:            cr.Name,
			ClientId:        clientID,
			ClientSecret:    clientSecret,
			Scopes:          cr.Spec.Scopes,
			ProviderOptions: providerOptions,
		})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// IDP was deleted externally, recreate.
				cr.Status.IdpId = ""
			} else {
				return "", fmt.Errorf("updating Google provider: %w", err)
			}
		} else {
			return cr.Status.IdpId, nil
		}
	}

	// Create new Google IDP.
	resp, err := r.Zitadel.Admin().AddGoogleProvider(ctx, &admin.AddGoogleProviderRequest{
		Name:            cr.Name,
		ClientId:        clientID,
		ClientSecret:    clientSecret,
		Scopes:          cr.Spec.Scopes,
		ProviderOptions: providerOptions,
	})
	if err != nil {
		return "", fmt.Errorf("creating Google provider: %w", err)
	}

	return resp.GetId(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IdentityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.IdentityProvider{}).
		Named("identityprovider").
		Complete(r)
}
