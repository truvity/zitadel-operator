package controller

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	policyv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/policy"
)

// DefaultNotificationPolicyReconciler reconciles a DefaultNotificationPolicy object.
type DefaultNotificationPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultnotificationpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultnotificationpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultnotificationpolicies/finalizers,verbs=update

func (r *DefaultNotificationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.DefaultNotificationPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Singleton conflict detection.
	if conflict, err := r.checkConflict(ctx, &cr); err != nil || conflict {
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	}

	// Deletion.
	if done, result, err := handleSingletonDeletion(ctx, r.Client, &cr, func() {
		_, _ = r.Zitadel.Admin().UpdateNotificationPolicy(ctx, &admin.UpdateNotificationPolicyRequest{
			PasswordChange: false,
		})
		logger.Info("reset instance notification policy to defaults (reset-on-delete annotation present)")
	}); done {
		return result, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Business logic.
	if err := r.reconcileSpec(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	if err := markReady(ctx, r.Client, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, false); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("defaultnotificationpolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *DefaultNotificationPolicyReconciler) checkConflict(ctx context.Context, cr *zitadelv1alpha2.DefaultNotificationPolicy) (bool, error) {
	var list zitadelv1alpha2.DefaultNotificationPolicyList
	if err := r.List(ctx, &list); err != nil {
		return false, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultNotificationPolicy") {
		_ = r.Status().Update(ctx, cr)
		return true, nil
	}
	return false, nil
}

func (r *DefaultNotificationPolicyReconciler) reconcileSpec(ctx context.Context, cr *zitadelv1alpha2.DefaultNotificationPolicy) error {
	logger := log.FromContext(ctx)
	current, err := r.Zitadel.Admin().GetNotificationPolicy(ctx, &admin.GetNotificationPolicyRequest{})
	if err != nil {
		return fmt.Errorf("getting default notification policy: %w", err)
	}
	if r.hasDrift(&cr.Spec, current.GetPolicy()) {
		passwordChange := false
		if cr.Spec.PasswordChange != nil {
			passwordChange = *cr.Spec.PasswordChange
		}
		_, err := r.Zitadel.Admin().UpdateNotificationPolicy(ctx, &admin.UpdateNotificationPolicyRequest{
			PasswordChange: passwordChange,
		})
		if err != nil {
			return fmt.Errorf("updating default notification policy: %w", err)
		}
		logger.Info("default notification policy updated (drift detected)")
	}
	return nil
}

// hasDrift checks if the current notification policy differs from the desired spec.
func (r *DefaultNotificationPolicyReconciler) hasDrift(spec *zitadelv1alpha2.DefaultNotificationPolicySpec, policy *policyv1.NotificationPolicy) bool {
	if policy == nil {
		return true
	}
	desiredPasswordChange := false
	if spec.PasswordChange != nil {
		desiredPasswordChange = *spec.PasswordChange
	}
	return desiredPasswordChange != policy.GetPasswordChange()
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultNotificationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultNotificationPolicy{}).
		Named("defaultnotificationpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
