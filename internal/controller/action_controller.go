package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// ActionReconciler reconciles an Action object.
type ActionReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=actions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=actions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=actions/finalizers,verbs=update

func (r *ActionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha1.Action
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.ActionId != "" {
			_, err := r.Zitadel.Management().DeleteAction(ctx, &management.DeleteActionRequest{
				Id: cr.Status.ActionId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("deleting action: %w", err)
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

	// Resolve timeout.
	timeout := 10
	if cr.Spec.Timeout > 0 {
		timeout = cr.Spec.Timeout
	}
	timeoutDuration := durationpb.New(time.Duration(timeout) * time.Second)

	// Create or update the action.
	actionID := cr.Status.ActionId
	if actionID == "" {
		// Try to find existing action by name.
		actionID = r.findActionByName(ctx, cr.Name)
	}

	if actionID == "" {
		// Create new action.
		resp, err := r.Zitadel.Management().CreateAction(ctx, &management.CreateActionRequest{
			Name:          cr.Name,
			Script:        cr.Spec.Script,
			Timeout:       timeoutDuration,
			AllowedToFail: cr.Spec.AllowedToFail,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("creating action: %w", err)
		}
		actionID = resp.GetId()
	} else {
		// Update existing action.
		_, err := r.Zitadel.Management().UpdateAction(ctx, &management.UpdateActionRequest{
			Id:            actionID,
			Name:          cr.Name,
			Script:        cr.Spec.Script,
			Timeout:       timeoutDuration,
			AllowedToFail: cr.Spec.AllowedToFail,
		})
		if err != nil {
			// Zitadel returns FailedPrecondition when the action hasn't changed — treat as success.
			if status.Code(err) != codes.FailedPrecondition {
				return ctrl.Result{}, fmt.Errorf("updating action: %w", err)
			}
		}
	}

	// Set trigger bindings.
	for _, trigger := range cr.Spec.Triggers {
		_, err := r.Zitadel.Management().SetTriggerActions(ctx, &management.SetTriggerActionsRequest{
			FlowType:    trigger.FlowType,
			TriggerType: trigger.TriggerType,
			ActionIds:   []string{actionID},
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("setting trigger actions (flow=%s, trigger=%s): %w", trigger.FlowType, trigger.TriggerType, err)
		}
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.ActionId = actionID
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	cr.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Synced",
			Message:            fmt.Sprintf("Action %q synced successfully", cr.Name),
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("action reconciled", "actionId", actionID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// findActionByName searches for an existing action by name.
func (r *ActionReconciler) findActionByName(ctx context.Context, name string) string {
	resp, err := r.Zitadel.Management().ListActions(ctx, &management.ListActionsRequest{})
	if err != nil {
		return ""
	}
	for _, a := range resp.GetResult() {
		if a.GetName() == name {
			return a.GetId()
		}
	}
	return ""
}

// SetupWithManager sets up the controller with the Manager.
func (r *ActionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.Action{}).
		Named("action").
		Complete(r)
}
