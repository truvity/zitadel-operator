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
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	policyv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/policy"
)

// DefaultPasswordComplexityPolicyReconciler reconciles a DefaultPasswordComplexityPolicy object.
type DefaultPasswordComplexityPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
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

	// Singleton conflict detection: only the earliest-created CR manages the instance.
	var list zitadelv1alpha2.DefaultPasswordComplexityPolicyList
	if err := r.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(&cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultPasswordComplexityPolicy") {
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if shouldResetOnDelete(&cr) {
			// Zitadel documented instance defaults: MinLength=8, HasLowercase=true, HasUppercase=true, HasNumber=true, HasSymbol=false.
			_, _ = r.Zitadel.Admin().UpdatePasswordComplexityPolicy(ctx, &admin.UpdatePasswordComplexityPolicyRequest{
				MinLength:    8,
				HasLowercase: true,
				HasUppercase: true,
				HasNumber:    true,
				HasSymbol:    false,
			})
			logger.Info("reset instance password complexity policy to defaults (reset-on-delete annotation present)")
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

	// Read current password complexity policy from Zitadel.
	current, err := r.Zitadel.Admin().GetPasswordComplexityPolicy(ctx, &admin.GetPasswordComplexityPolicyRequest{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting default password complexity policy: %w", err)
	}

	// Detect drift and update if needed.
	policy := current.GetPolicy()
	if r.hasDrift(&cr.Spec, policy) {
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
			return ctrl.Result{}, fmt.Errorf("updating default password complexity policy: %w", err)
		}
		logger.Info("default password complexity policy updated (drift detected)")
	}

	// Update status.
	if !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("defaultpasswordcomplexitypolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
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
