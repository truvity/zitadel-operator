package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	actionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/action/v2"
)

// ActionExecutionReconciler reconciles an ActionExecution object.
type ActionExecutionReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=actionexecutions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=actionexecutions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=actionexecutions/finalizers,verbs=update

func (r *ActionExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.ActionExecution
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion — set execution with empty targets to "unset" it.
	if !cr.DeletionTimestamp.IsZero() {
		condition := r.buildCondition(&cr)
		if condition != nil {
			_, _ = r.Zitadel.Action().SetExecution(ctx, &actionv2.SetExecutionRequest{
				Condition: condition,
				Targets:   nil, // Empty targets removes the execution.
			})
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

	// Resolve target IDs.
	targetIDs, err := r.resolveTargets(ctx, &cr)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for target ref to become ready", "error", err)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving targets: %w", err)
	}

	// Build condition.
	condition := r.buildCondition(&cr)
	if condition == nil {
		return ctrl.Result{}, fmt.Errorf("one of condition.function, condition.request, or condition.event is required")
	}

	// Set execution.
	_, err = r.Zitadel.Action().SetExecution(ctx, &actionv2.SetExecutionRequest{
		Condition: condition,
		Targets:   targetIDs,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("setting execution: %w", err)
	}

	// Update status.
	if !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.Ready = true
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		cr.Status.LastSyncTime = &now
		if err := applyStatus(ctx, r.Client, r.Config, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("actionexecution reconciled", "condition", cr.Spec.Condition, "targets", targetIDs)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ActionExecutionReconciler) resolveTargets(ctx context.Context, cr *zitadelv1alpha2.ActionExecution) ([]string, error) {
	var targetIDs []string
	for _, t := range cr.Spec.Targets {
		if t.TargetId != "" && t.TargetRef != nil {
			return nil, fmt.Errorf("targetId and targetRef are mutually exclusive")
		}
		if t.TargetId != "" {
			targetIDs = append(targetIDs, t.TargetId)
			continue
		}
		if t.TargetRef != nil {
			ns := t.TargetRef.Namespace
			if ns == "" {
				ns = cr.Namespace
			}
			var at zitadelv1alpha2.ActionTarget
			if err := r.Get(ctx, client.ObjectKey{Name: t.TargetRef.Name, Namespace: ns}, &at); err != nil {
				return nil, fmt.Errorf("resolving targetRef %s/%s: %w", ns, t.TargetRef.Name, err)
			}
			if at.Status.TargetId == "" {
				return nil, fmt.Errorf("targetRef %s/%s not yet ready (no targetId in status)", ns, t.TargetRef.Name)
			}
			targetIDs = append(targetIDs, at.Status.TargetId)
			continue
		}
		return nil, fmt.Errorf("each target must specify targetId or targetRef")
	}
	return targetIDs, nil
}

func (r *ActionExecutionReconciler) buildCondition(cr *zitadelv1alpha2.ActionExecution) *actionv2.Condition {
	c := cr.Spec.Condition
	switch {
	case c.Request != "":
		return &actionv2.Condition{
			ConditionType: &actionv2.Condition_Request{
				Request: &actionv2.RequestExecution{
					Condition: &actionv2.RequestExecution_Method{
						Method: c.Request,
					},
				},
			},
		}
	case c.Function != "":
		return &actionv2.Condition{
			ConditionType: &actionv2.Condition_Function{
				Function: &actionv2.FunctionExecution{
					Name: c.Function,
				},
			},
		}
	case c.Event != "":
		return &actionv2.Condition{
			ConditionType: &actionv2.Condition_Event{
				Event: &actionv2.EventExecution{
					Condition: &actionv2.EventExecution_Event{
						Event: c.Event,
					},
				},
			},
		}
	default:
		return nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ActionExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.ActionExecution{}).
		Named("actionexecution").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
