package controller

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/idp"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// IdentityProviderReconciler reconciles an IdentityProvider object.
type IdentityProviderReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
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
		if isRefNotReady(err) {
			logger.Info("waiting for organization ref to become ready", "error", err)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		if cr.Status.IdpId != "" {
			_, err := zclient(ctx, r.Zitadel).Management().DeleteProvider(ctx, &management.DeleteProviderRequest{
				Id: cr.Status.IdpId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return err
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

	// Ensure IDP exists.
	idpID, err := r.ensureIDP(ctx, &cr)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := cr.Status.IdpId != idpID
	cr.Status.IdpId = idpID
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
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
		_, err := zclient(ctx, r.Zitadel).Management().UpdateGenericOIDCProvider(ctx, &management.UpdateGenericOIDCProviderRequest{
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
	addResp, err := zclient(ctx, r.Zitadel).Management().AddGenericOIDCProvider(ctx, &management.AddGenericOIDCProviderRequest{
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
