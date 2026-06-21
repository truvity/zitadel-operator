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

// DefaultPasswordAgePolicyReconciler reconciles a DefaultPasswordAgePolicy object.
type DefaultPasswordAgePolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultpasswordagepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultpasswordagepolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultpasswordagepolicies/finalizers,verbs=update

func (r *DefaultPasswordAgePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.DefaultPasswordAgePolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Singleton conflict detection.
	if conflict, err := r.checkConflict(ctx, &cr); err != nil || conflict {
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	}

	// Deletion.
	if done, result, err := handleSingletonDeletion(ctx, r.Client, &cr, func() {
		_, _ = r.Zitadel.Admin().UpdatePasswordAgePolicy(ctx, &admin.UpdatePasswordAgePolicyRequest{
			MaxAgeDays:     0,
			ExpireWarnDays: 0,
		})
		logger.Info("reset instance password age policy to defaults (reset-on-delete annotation present)")
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

	logger.Info("defaultpasswordagepolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *DefaultPasswordAgePolicyReconciler) checkConflict(ctx context.Context, cr *zitadelv1alpha2.DefaultPasswordAgePolicy) (bool, error) {
	var list zitadelv1alpha2.DefaultPasswordAgePolicyList
	if err := r.List(ctx, &list); err != nil {
		return false, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultPasswordAgePolicy") {
		_ = r.Status().Update(ctx, cr)
		return true, nil
	}
	return false, nil
}

func (r *DefaultPasswordAgePolicyReconciler) reconcileSpec(ctx context.Context, cr *zitadelv1alpha2.DefaultPasswordAgePolicy) error {
	logger := log.FromContext(ctx)
	current, err := r.Zitadel.Admin().GetPasswordAgePolicy(ctx, &admin.GetPasswordAgePolicyRequest{})
	if err != nil {
		return fmt.Errorf("getting default password age policy: %w", err)
	}
	if r.hasDrift(&cr.Spec, current.GetPolicy()) {
		_, err := r.Zitadel.Admin().UpdatePasswordAgePolicy(ctx, &admin.UpdatePasswordAgePolicyRequest{
			MaxAgeDays:     cr.Spec.MaxAgeDays,
			ExpireWarnDays: cr.Spec.ExpireWarnDays,
		})
		if err != nil {
			return fmt.Errorf("updating default password age policy: %w", err)
		}
		logger.Info("default password age policy updated (drift detected)")
	}
	return nil
}

// hasDrift checks if the current password age policy differs from the desired spec.
func (r *DefaultPasswordAgePolicyReconciler) hasDrift(spec *zitadelv1alpha2.DefaultPasswordAgePolicySpec, policy *policyv1.PasswordAgePolicy) bool {
	if policy == nil {
		return true
	}
	if uint64(spec.MaxAgeDays) != policy.GetMaxAgeDays() {
		return true
	}
	if uint64(spec.ExpireWarnDays) != policy.GetExpireWarnDays() {
		return true
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultPasswordAgePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultPasswordAgePolicy{}).
		Named("defaultpasswordagepolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
