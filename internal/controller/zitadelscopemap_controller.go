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
)

// ZitadelScopeMapReconciler validates ZitadelScopeMap objects:
// instance match (fail-closed on mismatch), rule invariants, and
// organization resolution (ID authoritative; name drift = Event).
type ZitadelScopeMapReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	// Instance is the operator's binding domain.
	Instance string
	// Namespace is the operator namespace; maps elsewhere are ignored.
	Namespace string
	Recorder  record.EventRecorder
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=zitadelscopemaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=zitadelscopemaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

func (r *ZitadelScopeMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.ZitadelScopeMap
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if cr.Namespace != r.Namespace {
		logger.Info("ignoring ZitadelScopeMap outside operator namespace", "namespace", cr.Namespace)
		return ctrl.Result{}, nil
	}

	if !cr.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil // no external state to clean up
	}

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

	// Rule invariants.
	for i := range cr.Spec.Rules {
		if err := scopemap.ValidateRule(&cr.Spec.Rules[i]); err != nil {
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse,
				"InvalidRule", err.Error())
			ready = false
		}
	}

	// Organization resolution (only meaningful when the instance matches).
	resolvedOrgID := cr.Spec.OrganizationId
	if ready {
		var err error
		resolvedOrgID, err = r.resolveOrganization(ctx, &cr)
		if err != nil {
			setCondition(&cr.Status.Conditions, ConditionTypeOrganizationResolved, metav1.ConditionFalse,
				"OrganizationLookupFailed", err.Error())
			ready = false
		} else {
			setCondition(&cr.Status.Conditions, ConditionTypeOrganizationResolved, metav1.ConditionTrue,
				"OrganizationResolved", fmt.Sprintf("organization %q resolved to id %s", cr.Spec.Organization, resolvedOrgID))
		}
	}

	if ready {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue,
			"Validated", "scope map validated")
	}

	cr.Status.Ready = ready
	cr.Status.ResolvedOrganizationId = resolvedOrgID
	cr.Status.ObservedGeneration = cr.Generation
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// resolveOrganization returns the authoritative org ID.
// spec.organizationId wins when set; the actual org name is compared against
// spec.organization and drift is reported as an Event (not an error).
// Without an ID, the org is looked up by exact name.
func (r *ZitadelScopeMapReconciler) resolveOrganization(ctx context.Context, cr *zitadelv1alpha2.ZitadelScopeMap) (string, error) {
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
		actual := resp.GetResult()[0].GetName()
		if actual != cr.Spec.Organization && r.Recorder != nil {
			r.Recorder.Eventf(cr, corev1.EventTypeWarning, "OrganizationNameDrift",
				"spec.organization %q differs from actual name %q of org %s (ID is authoritative)",
				cr.Spec.Organization, actual, cr.Spec.OrganizationId)
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
func (r *ZitadelScopeMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.ZitadelScopeMap{}).
		Named("zitadelscopemap").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
