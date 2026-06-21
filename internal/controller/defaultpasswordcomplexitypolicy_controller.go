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

	// Handle deletion: reset to safe defaults.
	if !cr.DeletionTimestamp.IsZero() {
		_, _ = r.Zitadel.Admin().UpdatePasswordComplexityPolicy(ctx, &admin.UpdatePasswordComplexityPolicyRequest{
			MinLength:    8,
			HasLowercase: true,
			HasUppercase: true,
			HasNumber:    true,
			HasSymbol:    false,
		})
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
	drifted := false
	if policy != nil {
		if cr.Spec.MinLength != policy.GetMinLength() {
			drifted = true
		}
		if cr.Spec.HasLowercase != policy.GetHasLowercase() {
			drifted = true
		}
		if cr.Spec.HasUppercase != policy.GetHasUppercase() {
			drifted = true
		}
		if cr.Spec.HasNumber != policy.GetHasNumber() {
			drifted = true
		}
		if cr.Spec.HasSymbol != policy.GetHasSymbol() {
			drifted = true
		}
	} else {
		drifted = true
	}

	if drifted {
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

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultPasswordComplexityPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultPasswordComplexityPolicy{}).
		Named("defaultpasswordcomplexitypolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
