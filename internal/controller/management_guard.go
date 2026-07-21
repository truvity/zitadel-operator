package controller

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/truvity/zitadel-operator/internal/config"
)

const (
	// AnnotationManagedBy records which operator deployment manages a tenant
	// CR (v0.19 ForeignManager guard). It is stamped at adoption — the first
	// successful entry into reconciliation — with the operator's management
	// identity (config.ManagementIdentity). Another operator that serves the
	// same instance (identical SSA field manager, so invisible to the v0.18
	// dual-serving gate) but carries a different management identity sets a
	// ForeignManager condition and does not reconcile the CR.
	//
	// Break-glass: if the owning operator is decommissioned, transfer or
	// remove the annotation manually (kubectl annotate <kind> <name>
	// zitadel.truvity.io/managed-by-) so another operator can adopt the CR.
	AnnotationManagedBy = "zitadel.truvity.io/managed-by"

	// ConditionTypeForeignManager is set (True) by a non-owning operator when
	// the CR's managed-by annotation names a different operator deployment.
	// The owning operator is unaffected; the condition is written with a
	// separate guard field manager so it never collides with the owner's
	// status ownership.
	ConditionTypeForeignManager = "ForeignManager"

	// guardFieldManagerPrefix must NOT share the "zitadel-operator" prefix:
	// foreignOperatorManagerPresent scans for that prefix to detect
	// dual-serving, and the guard condition write must not look like a second
	// serving operator.
	guardFieldManagerPrefix = "zitadel-guard/"
)

// guardFieldManager returns the SSA field manager for ForeignManager
// condition writes. It embeds the management identity so two non-owning
// operators each own their own write (deterministic content prevents
// flapping).
func guardFieldManager(cfg *config.Config) string {
	return guardFieldManagerPrefix + cfg.ManagementIdentity()
}

// managementGate implements the v0.19 ForeignManager guard for tenant CRs.
// It runs after the dual-serving instance gate (which handles operators of
// DIFFERENT instances); this gate protects against two operators of the SAME
// instance whose namespace selections overlap — their SSA field managers are
// identical, so the instance gate cannot see them.
//
//   - annotation absent: stamp this operator's management identity (adoption)
//     and proceed. During deletion nothing is stamped — legacy CRs deleted
//     before ever being adopted just proceed to cleanup.
//   - annotation matches: proceed (releasing a previously written guard
//     condition, if any).
//   - annotation differs: write a ForeignManager condition via the guard
//     field manager, skip reconciliation entirely (protective default), and
//     re-check on the periodic interval. This also holds during deletion:
//     same-instance foreign deletes are NOT server-side no-ops, so cleanup
//     and finalizer removal are left to the owning operator.
func managementGate(ctx context.Context, k8s client.Client, cfg *config.Config, obj client.Object, conditions *[]metav1.Condition) (done bool, result ctrl.Result, err error) {
	if cfg == nil {
		return false, ctrl.Result{}, nil
	}
	identity := cfg.ManagementIdentity()
	current := obj.GetAnnotations()[AnnotationManagedBy]
	deleting := !obj.GetDeletionTimestamp().IsZero()

	switch current {
	case identity:
		// Owner. Release a stale guard condition from a previous foreign
		// phase of this operator (annotation transferred to us).
		if hasConditionType(*conditions, ConditionTypeForeignManager) {
			if err := applyGuardCondition(ctx, k8s, cfg, obj, *conditions, nil); err != nil {
				return true, ctrl.Result{}, err
			}
		}
		return false, ctrl.Result{}, nil

	case "":
		if deleting {
			// Never adopt a CR on its way out.
			return false, ctrl.Result{}, nil
		}
		// Adoption: stamp our management identity. A plain Update surfaces
		// conflicts, so two same-instance operators racing for an unstamped
		// CR converge: the loser errors, requeues, and then observes the
		// winner's identity.
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[AnnotationManagedBy] = identity
		obj.SetAnnotations(annotations)
		if err := k8s.Update(ctx, obj); err != nil {
			return true, ctrl.Result{}, err
		}
		log.FromContext(ctx).Info("adopted resource for management", "managedBy", identity)
		return false, ctrl.Result{}, nil

	default:
		// Foreign: another operator deployment of the same instance manages
		// this CR. Hands off — including deletion.
		cond := &metav1.Condition{
			Type:   ConditionTypeForeignManager,
			Status: metav1.ConditionTrue,
			Reason: "ManagedByOtherOperator",
			// Deterministic (mentions only the owner) so several non-owning
			// operators co-writing the condition converge instead of flapping.
			Message: "resource is managed by operator " + current +
				" (annotation " + AnnotationManagedBy + "); transfer or remove the annotation to change ownership",
		}
		if err := applyGuardCondition(ctx, k8s, cfg, obj, *conditions, cond); err != nil {
			return true, ctrl.Result{}, err
		}
		log.FromContext(ctx).Info("foreign-managed resource, skipping (ForeignManager)",
			"managedBy", current, "self", identity)
		return true, ctrl.Result{RequeueAfter: requeueInterval}, nil
	}
}

// hasConditionType reports whether a condition of the given type is recorded.
func hasConditionType(conditions []metav1.Condition, condType string) bool {
	for _, c := range conditions {
		if c.Type == condType {
			return true
		}
	}
	return false
}

// applyGuardCondition SSA-writes (cond != nil) or releases (cond == nil) the
// ForeignManager condition under the guard field manager. The patch contains
// ONLY that condition, so the guard manager never claims ownership of the
// owning operator's status fields; conditions are listType=map keyed by type,
// so entries from other managers survive. The observed conditions are used to
// reuse an unchanged entry's transition time so repeated applies do not churn
// the object.
func applyGuardCondition(ctx context.Context, k8s client.Client, cfg *config.Config, obj client.Object, observed []metav1.Condition, cond *metav1.Condition) error {
	gvk, err := apiutil.GVKForObject(obj, k8s.Scheme())
	if err != nil {
		return err
	}
	patch := &unstructured.Unstructured{Object: map[string]interface{}{}}
	patch.SetGroupVersionKind(gvk)
	patch.SetName(obj.GetName())
	patch.SetNamespace(obj.GetNamespace())
	status := map[string]interface{}{}
	if cond != nil {
		transition := metav1.Now()
		for _, c := range observed {
			if c.Type == cond.Type && c.Status == cond.Status && c.Reason == cond.Reason {
				transition = c.LastTransitionTime
				break
			}
		}
		status["conditions"] = []interface{}{map[string]interface{}{
			"type":               cond.Type,
			"status":             string(cond.Status),
			"reason":             cond.Reason,
			"message":            cond.Message,
			"lastTransitionTime": transition.UTC().Format(metav1.RFC3339Micro),
			"observedGeneration": obj.GetGeneration(),
		}}
	}
	patch.Object["status"] = status
	return k8s.Status().Patch(ctx, patch, client.Apply, //nolint:staticcheck // SA1019: unstructured apply-patch is the stable path for a minimal status-only SSA document (see applyStatus)
		client.FieldOwner(guardFieldManager(cfg)), client.ForceOwnership)
}
