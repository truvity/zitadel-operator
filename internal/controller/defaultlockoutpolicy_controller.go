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

	// Handle deletion: reset to safe defaults.
	if !cr.DeletionTimestamp.IsZero() {
		_, _ = r.Zitadel.Admin().UpdateLockoutPolicy(ctx, &admin.UpdateLockoutPolicyRequest{
			MaxPasswordAttempts: 0,
			MaxOtpAttempts:      0,
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

	// Read current lockout policy from Zitadel.
	current, err := r.Zitadel.Admin().GetLockoutPolicy(ctx, &admin.GetLockoutPolicyRequest{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting default lockout policy: %w", err)
	}

	// Detect drift and update if needed.
	policy := current.GetPolicy()
	drifted := false
	if policy != nil {
		if uint64(cr.Spec.MaxPasswordAttempts) != policy.GetMaxPasswordAttempts() {
			drifted = true
		}
		if uint64(cr.Spec.MaxOtpAttempts) != policy.GetMaxOtpAttempts() {
			drifted = true
		}
	} else {
		drifted = true
	}

	if drifted {
		_, err := r.Zitadel.Admin().UpdateLockoutPolicy(ctx, &admin.UpdateLockoutPolicyRequest{
			MaxPasswordAttempts: cr.Spec.MaxPasswordAttempts,
			MaxOtpAttempts:      cr.Spec.MaxOtpAttempts,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("updating default lockout policy: %w", err)
		}
		logger.Info("default lockout policy updated (drift detected)")
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

	logger.Info("defaultlockoutpolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultLockoutPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultLockoutPolicy{}).
		Named("defaultlockoutpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
