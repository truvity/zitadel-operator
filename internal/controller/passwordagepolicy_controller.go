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

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// PasswordAgePolicyReconciler reconciles a PasswordAgePolicy object.
type PasswordAgePolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordagepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordagepolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordagepolicies/finalizers,verbs=update

func (r *PasswordAgePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.PasswordAgePolicy
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

	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		_, err := zclient(ctx, r.Zitadel).Management().ResetPasswordAgePolicyToDefault(ctx, &management.ResetPasswordAgePolicyToDefaultRequest{})
		if err != nil && status.Code(err) != codes.NotFound {
			return nil // non-critical
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

	// Business logic.
	if err := r.reconcileSpec(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := cr.Status.OrganizationId != orgID
	cr.Status.OrganizationId = orgID
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("passwordagepolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *PasswordAgePolicyReconciler) reconcileSpec(ctx context.Context, cr *zitadelv1alpha2.PasswordAgePolicy) error {
	_, err := zclient(ctx, r.Zitadel).Management().UpdateCustomPasswordAgePolicy(ctx, &management.UpdateCustomPasswordAgePolicyRequest{
		MaxAgeDays:     cr.Spec.MaxAgeDays,
		ExpireWarnDays: cr.Spec.ExpireWarnDays,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			_, err = zclient(ctx, r.Zitadel).Management().AddCustomPasswordAgePolicy(ctx, &management.AddCustomPasswordAgePolicyRequest{
				MaxAgeDays:     cr.Spec.MaxAgeDays,
				ExpireWarnDays: cr.Spec.ExpireWarnDays,
			})
			if err != nil {
				return fmt.Errorf("adding custom password age policy: %w", err)
			}
		} else {
			return fmt.Errorf("updating custom password age policy: %w", err)
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PasswordAgePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.PasswordAgePolicy{}).
		Named("passwordagepolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
