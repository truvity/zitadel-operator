package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	objectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object/v2"
	orgv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/org/v2"
)

const (
	// ConditionTypeInstanceMatch reports whether spec.instance matches the
	// operator binding. False = fail-closed for every namespace the map serves.
	ConditionTypeInstanceMatch = "InstanceMatch"

	// ConditionTypeOrganizationResolved reports org name/ID resolution state.
	ConditionTypeOrganizationResolved = "OrganizationResolved"

	// ConditionTypeBindingContained reports whether the map's organization is
	// within the operator binding's reach. False/ForeignOrganization is the
	// long-lived visible state for org-owner bindings serving a foreign-org
	// map (the Event alone would expire after ~1h).
	ConditionTypeBindingContained = "BindingContained"

	// ConditionTypeOrganizationNameDrift is True while spec.organization
	// disagrees with the actual org name of the authoritative
	// spec.organizationId (long-lived drift is a condition, not just an
	// expiring Event).
	ConditionTypeOrganizationNameDrift = "OrganizationNameDrift"
)

// ScopeMapReconciler validates ScopeMap objects:
// instance match (fail-closed on mismatch), rule invariants, and
// organization resolution (ID authoritative; name drift = Event).
// Every reconcile (including map deletion) triggers a delegation sweep so
// delegates are revoked eagerly when their scope stops matching.
type ScopeMapReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
	// Instance is the operator's binding domain.
	Instance string
	// Namespace is the operator namespace; maps elsewhere are ignored.
	Namespace string
	Recorder  record.EventRecorder
	// GC sweeps delegates after map changes (eager revoke on unmatch).
	GC *delegation.GC
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=scopemaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=scopemaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

func (r *ScopeMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.ScopeMap
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Map deleted: sweep so delegates of vanished scopes are revoked.
			r.sweep(ctx)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if cr.Namespace != r.Namespace {
		logger.Info("ignoring ScopeMap outside operator namespace", "namespace", cr.Namespace)
		return ctrl.Result{}, nil
	}

	if !cr.DeletionTimestamp.IsZero() {
		// No external state on the map itself; delegates are cleaned up by
		// the sweep once the object is gone.
		return ctrl.Result{}, nil
	}

	ready := r.validateMap(ctx, &cr)

	// Organization resolution (only meaningful when validation passed).
	resolvedOrgID := cr.Spec.OrganizationId
	if ready {
		resolvedOrgID, ready = r.recordOrganizationResolution(ctx, &cr)
	}

	// INF-424: an org-owner binding may only serve maps for its own org.
	if !r.recordBindingContainment(&cr, resolvedOrgID, ready) {
		ready = false
	}

	if ready {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue,
			"Validated", "scope map validated")
	}

	cr.Status.Ready = ready
	cr.Status.ResolvedOrganizationId = resolvedOrgID
	cr.Status.ObservedGeneration = cr.Generation
	if err := applyStatus(ctx, r.Client, r.Config, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Map changed: eagerly sweep delegates (rules may have been removed).
	r.sweep(ctx)

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// recordOrganizationResolution resolves the map's org and records the
// OrganizationResolved condition. Returns the resolved ID and whether the
// map is still ready.
func (r *ScopeMapReconciler) recordOrganizationResolution(ctx context.Context, cr *zitadelv1alpha2.ScopeMap) (string, bool) {
	resolvedOrgID, err := r.resolveOrganization(ctx, cr)
	if err != nil {
		setCondition(&cr.Status.Conditions, ConditionTypeOrganizationResolved, metav1.ConditionFalse,
			"OrganizationLookupFailed", err.Error())
		return cr.Spec.OrganizationId, false
	}
	setCondition(&cr.Status.Conditions, ConditionTypeOrganizationResolved, metav1.ConditionTrue,
		"OrganizationResolved", fmt.Sprintf("organization %q resolved to id %s", cr.Spec.Organization, resolvedOrgID))
	return resolvedOrgID, true
}

// recordBindingContainment enforces the INF-424 org-owner containment rule:
// a foreign-org map is rejected (BindingContained=False + Ready=False +
// Warning Event). Returns false when the map must be marked not ready.
func (r *ScopeMapReconciler) recordBindingContainment(cr *zitadelv1alpha2.ScopeMap, resolvedOrgID string, ready bool) bool {
	if r.Config == nil || r.Config.Binding != config.BindingOrgOwner || !ready {
		return true
	}
	if r.Config.BoundOrganizationId != "" && resolvedOrgID != r.Config.BoundOrganizationId {
		msg := fmt.Sprintf("map organization %s is foreign to the org-owner binding (bound org %s); all rules fail-closed",
			resolvedOrgID, r.Config.BoundOrganizationId)
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse,
			"NotSupportedAtBindingLevel", msg)
		setCondition(&cr.Status.Conditions, ConditionTypeBindingContained, metav1.ConditionFalse,
			"ForeignOrganization", msg)
		if r.Recorder != nil {
			r.Recorder.Event(cr, corev1.EventTypeWarning, "ForeignOrganization", msg)
		}
		return false
	}
	setCondition(&cr.Status.Conditions, ConditionTypeBindingContained, metav1.ConditionTrue,
		"Contained", "map organization is within the org-owner binding")
	return true
}

// validateMap checks instance match and rule invariants, recording conditions.
func (r *ScopeMapReconciler) validateMap(_ context.Context, cr *zitadelv1alpha2.ScopeMap) bool {
	ready := true

	// Instance check: fail-closed on mismatch.
	if cr.Spec.Instance != r.Instance {
		setCondition(&cr.Status.Conditions, ConditionTypeInstanceMatch, metav1.ConditionFalse,
			"InstanceMismatch",
			fmt.Sprintf("spec.instance %q does not match operator binding %q; all rules fail-closed", cr.Spec.Instance, r.Instance))
		ready = false
	} else {
		setCondition(&cr.Status.Conditions, ConditionTypeInstanceMatch, metav1.ConditionTrue,
			"InstanceMatch", "spec.instance matches the operator binding")
	}

	// Organization identity: at least one of name / ID.
	if cr.Spec.Organization == "" && cr.Spec.OrganizationId == "" {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse,
			"InvalidSpec", "one of spec.organization or spec.organizationId must be set")
		ready = false
	}

	// Rule invariants.
	for i := range cr.Spec.Rules {
		if err := scopemap.ValidateRule(&cr.Spec.Rules[i]); err != nil {
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse,
				"InvalidRule", err.Error())
			ready = false
		}
	}
	return ready
}

// sweep triggers an eager delegation sweep, tolerating transient failures
// (the periodic GC loop catches anything missed here).
func (r *ScopeMapReconciler) sweep(ctx context.Context) {
	if r.GC == nil {
		return
	}
	if err := r.GC.SweepOnce(ctx); err != nil {
		log.FromContext(ctx).Info("delegation sweep after map change failed (periodic GC will retry)", "error", err)
	}
}

// resolveOrganization returns the authoritative org ID.
// spec.organizationId wins when set; the actual org name is compared against
// spec.organization and drift is reported as an Event (not an error).
// Without an ID, the org is looked up by exact name.
func (r *ScopeMapReconciler) resolveOrganization(ctx context.Context, cr *zitadelv1alpha2.ScopeMap) (string, error) {
	if cr.Spec.OrganizationId != "" {
		resp, err := r.Zitadel.Organization().ListOrganizations(ctx, &orgv2.ListOrganizationsRequest{
			Queries: []*orgv2.SearchQuery{{
				Query: &orgv2.SearchQuery_IdQuery{
					IdQuery: &orgv2.OrganizationIDQuery{Id: cr.Spec.OrganizationId},
				},
			}},
		})
		if err != nil {
			return "", fmt.Errorf("looking up organization %s: %w", cr.Spec.OrganizationId, err)
		}
		if len(resp.GetResult()) == 0 {
			return "", fmt.Errorf("organization id %s not found", cr.Spec.OrganizationId)
		}
		// Name drift: the ID is authoritative; a differing (non-empty) name
		// is long-lived visible state, so it is a condition as well as an
		// Event (Events expire after ~1h).
		actual := resp.GetResult()[0].GetName()
		if cr.Spec.Organization != "" && actual != cr.Spec.Organization {
			setCondition(&cr.Status.Conditions, ConditionTypeOrganizationNameDrift, metav1.ConditionTrue,
				"NameDrift",
				fmt.Sprintf("spec.organization %q differs from actual name %q of org %s (ID is authoritative)",
					cr.Spec.Organization, actual, cr.Spec.OrganizationId))
			if r.Recorder != nil {
				r.Recorder.Eventf(cr, corev1.EventTypeWarning, "OrganizationNameDrift",
					"spec.organization %q differs from actual name %q of org %s (ID is authoritative)",
					cr.Spec.Organization, actual, cr.Spec.OrganizationId)
			}
		} else {
			setCondition(&cr.Status.Conditions, ConditionTypeOrganizationNameDrift, metav1.ConditionFalse,
				"InSync", "spec.organization matches the actual organization name")
		}
		return cr.Spec.OrganizationId, nil
	}

	resp, err := r.Zitadel.Organization().ListOrganizations(ctx, &orgv2.ListOrganizationsRequest{
		Queries: []*orgv2.SearchQuery{{
			Query: &orgv2.SearchQuery_NameQuery{
				NameQuery: &orgv2.OrganizationNameQuery{
					Name:   cr.Spec.Organization,
					Method: objectv2.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
				},
			},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("looking up organization %q by name: %w", cr.Spec.Organization, err)
	}
	for _, o := range resp.GetResult() {
		if o.GetName() == cr.Spec.Organization {
			return o.GetId(), nil
		}
	}
	return "", fmt.Errorf("organization %q not found by name", cr.Spec.Organization)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScopeMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.ScopeMap{}).
		Named("scopemap").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
