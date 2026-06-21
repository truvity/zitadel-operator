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

// DefaultLockoutPolicyReconciler reconciles a DefaultLockoutPolicy object.
type DefaultLockoutPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultlockoutpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultlockoutpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultlockoutpolicies/finalizers,verbs=update

func (r *DefaultLockoutPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.DefaultLockoutPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Singleton conflict detection.
	if conflict, err := r.checkConflict(ctx, &cr); err != nil || conflict {
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	}

	// Deletion.
	if done, result, err := handleSingletonDeletion(ctx, r.Client, &cr, func() {
		_, _ = r.Zitadel.Admin().UpdateLockoutPolicy(ctx, &admin.UpdateLockoutPolicyRequest{
			MaxPasswordAttempts: 0,
			MaxOtpAttempts:      0,
		})
		logger.Info("reset instance lockout policy to defaults (reset-on-delete annotation present)")
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

	logger.Info("defaultlockoutpolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *DefaultLockoutPolicyReconciler) checkConflict(ctx context.Context, cr *zitadelv1alpha2.DefaultLockoutPolicy) (bool, error) {
	var list zitadelv1alpha2.DefaultLockoutPolicyList
	if err := r.List(ctx, &list); err != nil {
		return false, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultLockoutPolicy") {
		_ = r.Status().Update(ctx, cr)
		return true, nil
	}
	return false, nil
}

func (r *DefaultLockoutPolicyReconciler) reconcileSpec(ctx context.Context, cr *zitadelv1alpha2.DefaultLockoutPolicy) error {
	logger := log.FromContext(ctx)
	current, err := r.Zitadel.Admin().GetLockoutPolicy(ctx, &admin.GetLockoutPolicyRequest{})
	if err != nil {
		return fmt.Errorf("getting default lockout policy: %w", err)
	}
	if r.hasDrift(&cr.Spec, current.GetPolicy()) {
		_, err := r.Zitadel.Admin().UpdateLockoutPolicy(ctx, &admin.UpdateLockoutPolicyRequest{
			MaxPasswordAttempts: cr.Spec.MaxPasswordAttempts,
			MaxOtpAttempts:      cr.Spec.MaxOtpAttempts,
		})
		if err != nil {
			return fmt.Errorf("updating default lockout policy: %w", err)
		}
		logger.Info("default lockout policy updated (drift detected)")
	}
	return nil
}

// hasDrift checks if the current lockout policy differs from the desired spec.
func (r *DefaultLockoutPolicyReconciler) hasDrift(spec *zitadelv1alpha2.DefaultLockoutPolicySpec, policy *policyv1.LockoutPolicy) bool {
	if policy == nil {
		return true
	}
	if uint64(spec.MaxPasswordAttempts) != policy.GetMaxPasswordAttempts() {
		return true
	}
	if uint64(spec.MaxOtpAttempts) != policy.GetMaxOtpAttempts() {
		return true
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultLockoutPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultLockoutPolicy{}).
		Named("defaultlockoutpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
