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

// LabelPolicyReconciler reconciles a LabelPolicy object.
type LabelPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=labelpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=labelpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=labelpolicies/finalizers,verbs=update

func (r *LabelPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.LabelPolicy
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
		_, err := zclient(ctx, r.Zitadel).Management().ResetLabelPolicyToDefault(ctx, &management.ResetLabelPolicyToDefaultRequest{})
		if err != nil && status.Code(err) != codes.NotFound {
			return err
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
	if err := r.ensureLabelPolicy(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Activate the label policy to make it live.
	_, err = zclient(ctx, r.Zitadel).Management().ActivateCustomLabelPolicy(ctx, &management.ActivateCustomLabelPolicyRequest{})
	if err != nil && status.Code(err) != codes.FailedPrecondition {
		logger.Info("could not activate label policy (may already be active)", "error", err)
	}

	// Status.
	statusChanged := cr.Status.OrganizationId != orgID
	cr.Status.OrganizationId = orgID
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("labelpolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *LabelPolicyReconciler) ensureLabelPolicy(ctx context.Context, cr *zitadelv1alpha2.LabelPolicy) error {
	fields := cr.Spec.LabelPolicyFields

	// Try to update existing custom label policy first.
	_, err := zclient(ctx, r.Zitadel).Management().UpdateCustomLabelPolicy(ctx, &management.UpdateCustomLabelPolicyRequest{
		PrimaryColor:        fields.PrimaryColor,
		BackgroundColor:     fields.BackgroundColor,
		WarnColor:           fields.WarnColor,
		FontColor:           fields.FontColor,
		PrimaryColorDark:    fields.PrimaryColorDark,
		BackgroundColorDark: fields.BackgroundColorDark,
		WarnColorDark:       fields.WarnColorDark,
		FontColorDark:       fields.FontColorDark,
		HideLoginNameSuffix: fields.HideLoginNameSuffix,
		DisableWatermark:    fields.DisableWatermark,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// No custom policy exists, create one.
			_, err = zclient(ctx, r.Zitadel).Management().AddCustomLabelPolicy(ctx, &management.AddCustomLabelPolicyRequest{
				PrimaryColor:        fields.PrimaryColor,
				BackgroundColor:     fields.BackgroundColor,
				WarnColor:           fields.WarnColor,
				FontColor:           fields.FontColor,
				PrimaryColorDark:    fields.PrimaryColorDark,
				BackgroundColorDark: fields.BackgroundColorDark,
				WarnColorDark:       fields.WarnColorDark,
				FontColorDark:       fields.FontColorDark,
				HideLoginNameSuffix: fields.HideLoginNameSuffix,
				DisableWatermark:    fields.DisableWatermark,
			})
			if err != nil {
				return fmt.Errorf("adding custom label policy: %w", err)
			}
		} else {
			return fmt.Errorf("updating custom label policy: %w", err)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LabelPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.LabelPolicy{}).
		Named("labelpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
