package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/idp"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// IdentityProviderReconciler reconciles an IdentityProvider object.
type IdentityProviderReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=identityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=identityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=identityproviders/finalizers,verbs=update

func (r *IdentityProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.IdentityProvider
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve organization.
	orgID, err := resolveOrganizationId(ctx, r.Client, r.Config, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for organization ref to become ready", "error", err)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.IdpId != "" {
			_, err := r.Zitadel.Management().DeleteProvider(ctx, &management.DeleteProviderRequest{
				Id: cr.Status.IdpId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("deleting identity provider: %w", err)
			}
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

	// Ensure IDP exists.
	idpID, err := r.ensureIDP(ctx, &cr)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	if cr.Status.IdpId != idpID || !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.IdpId = idpID
		cr.Status.Ready = true
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		cr.Status.LastSyncTime = &now
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("identityprovider reconciled", "idpId", idpID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *IdentityProviderReconciler) ensureIDP(ctx context.Context, cr *zitadelv1alpha2.IdentityProvider) (string, error) {
	providerOptions := &idp.Options{
		IsLinkingAllowed:  cr.Spec.IsLinkingAllowed,
		IsCreationAllowed: cr.Spec.IsAutoCreation,
		IsAutoCreation:    cr.Spec.IsAutoCreation,
		IsAutoUpdate:      cr.Spec.IsAutoUpdate,
	}

	// If we already have an IDP ID, update it.
	if cr.Status.IdpId != "" {
		_, err := r.Zitadel.Management().UpdateGenericOIDCProvider(ctx, &management.UpdateGenericOIDCProviderRequest{
			Id:              cr.Status.IdpId,
			Name:            cr.Spec.Name,
			Issuer:          cr.Spec.Issuer,
			ClientId:        cr.Spec.ClientId,
			ClientSecret:    cr.Spec.ClientSecret,
			Scopes:          cr.Spec.Scopes,
			ProviderOptions: providerOptions,
		})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// IDP was deleted externally, recreate below.
			} else {
				return "", fmt.Errorf("updating identity provider: %w", err)
			}
		} else {
			return cr.Status.IdpId, nil
		}
	}

	// Create new IDP.
	addResp, err := r.Zitadel.Management().AddGenericOIDCProvider(ctx, &management.AddGenericOIDCProviderRequest{
		Name:            cr.Spec.Name,
		Issuer:          cr.Spec.Issuer,
		ClientId:        cr.Spec.ClientId,
		ClientSecret:    cr.Spec.ClientSecret,
		Scopes:          cr.Spec.Scopes,
		ProviderOptions: providerOptions,
	})
	if err != nil {
		return "", fmt.Errorf("adding identity provider: %w", err)
	}

	return addResp.GetId(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IdentityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.IdentityProvider{}).
		Named("identityprovider").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
