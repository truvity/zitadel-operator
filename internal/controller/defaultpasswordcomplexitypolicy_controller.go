package controller

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	policyv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/policy"
)

// DefaultPasswordComplexityPolicyReconciler reconciles a DefaultPasswordComplexityPolicy object.
type DefaultPasswordComplexityPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultpasswordcomplexitypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultpasswordcomplexitypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultpasswordcomplexitypolicies/finalizers,verbs=update

func (r *DefaultPasswordComplexityPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.DefaultPasswordComplexityPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// INF-424 degradation matrix: instance-level resources are not supported
	// under an org-owner binding (no-op during deletion so finalizers complete).
	if done, result, err := checkBindingLevel(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, &cr.Status.Ready); done {
		return result, err
	}

	// Singleton conflict detection.
	if conflict, err := r.checkConflict(ctx, &cr); err != nil || conflict {
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	}

	// Deletion.
	if done, result, err := handleSingletonDeletion(ctx, r.Client, &cr, func() {
		_, _ = r.Zitadel.Admin().UpdatePasswordComplexityPolicy(ctx, &admin.UpdatePasswordComplexityPolicyRequest{
			MinLength:    8,
			HasLowercase: true,
			HasUppercase: true,
			HasNumber:    true,
			HasSymbol:    false,
		})
		logger.Info("reset instance password complexity policy to defaults (reset-on-delete annotation present)")
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
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, false); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("defaultpasswordcomplexitypolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *DefaultPasswordComplexityPolicyReconciler) checkConflict(ctx context.Context, cr *zitadelv1alpha2.DefaultPasswordComplexityPolicy) (bool, error) {
	var list zitadelv1alpha2.DefaultPasswordComplexityPolicyList
	if err := r.List(ctx, &list); err != nil {
		return false, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultPasswordComplexityPolicy") {
		_ = applyStatus(ctx, r.Client, r.Config, cr)
		return true, nil
	}
	return false, nil
}

func (r *DefaultPasswordComplexityPolicyReconciler) reconcileSpec(ctx context.Context, cr *zitadelv1alpha2.DefaultPasswordComplexityPolicy) error {
	logger := log.FromContext(ctx)
	current, err := r.Zitadel.Admin().GetPasswordComplexityPolicy(ctx, &admin.GetPasswordComplexityPolicyRequest{})
	if err != nil {
		return fmt.Errorf("getting default password complexity policy: %w", err)
	}
	if r.hasDrift(&cr.Spec, current.GetPolicy()) {
		minLength := cr.Spec.MinLength
		if minLength > uint64(^uint32(0)) {
			minLength = uint64(^uint32(0))
		}
		_, err := r.Zitadel.Admin().UpdatePasswordComplexityPolicy(ctx, &admin.UpdatePasswordComplexityPolicyRequest{
			MinLength:    uint32(minLength),
			HasLowercase: cr.Spec.HasLowercase,
			HasUppercase: cr.Spec.HasUppercase,
			HasNumber:    cr.Spec.HasNumber,
			HasSymbol:    cr.Spec.HasSymbol,
		})
		if err != nil {
			return fmt.Errorf("updating default password complexity policy: %w", err)
		}
		logger.Info("default password complexity policy updated (drift detected)")
	}
	return nil
}

// hasDrift checks if the current password complexity policy differs from the desired spec.
func (r *DefaultPasswordComplexityPolicyReconciler) hasDrift(spec *zitadelv1alpha2.DefaultPasswordComplexityPolicySpec, policy *policyv1.PasswordComplexityPolicy) bool {
	if policy == nil {
		return true
	}
	if spec.MinLength != policy.GetMinLength() {
		return true
	}
	if spec.HasLowercase != policy.GetHasLowercase() {
		return true
	}
	if spec.HasUppercase != policy.GetHasUppercase() {
		return true
	}
	if spec.HasNumber != policy.GetHasNumber() {
		return true
	}
	if spec.HasSymbol != policy.GetHasSymbol() {
		return true
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultPasswordComplexityPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultPasswordComplexityPolicy{}).
		Named("defaultpasswordcomplexitypolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
