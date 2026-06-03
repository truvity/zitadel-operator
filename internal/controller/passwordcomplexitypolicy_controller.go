package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
)

// PasswordComplexityPolicyReconciler reconciles a PasswordComplexityPolicy object.
type PasswordComplexityPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordcomplexitypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordcomplexitypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordcomplexitypolicies/finalizers,verbs=update

func (r *PasswordComplexityPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha1.PasswordComplexityPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion — no reset available at instance level, just remove finalizer.
	if !cr.DeletionTimestamp.IsZero() {
		logger.Info("passwordcomplexitypolicy deleted, instance-level policy remains as-is")
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

	// Update password complexity policy in Zitadel.
	_, err := r.Zitadel.Admin().UpdatePasswordComplexityPolicy(ctx, &admin.UpdatePasswordComplexityPolicyRequest{
		MinLength:    uint32(cr.Spec.MinLength), //nolint:gosec // value range validated by k8s schema
		HasUppercase: cr.Spec.HasUppercase,
		HasLowercase: cr.Spec.HasLowercase,
		HasNumber:    cr.Spec.HasNumber,
		HasSymbol:    cr.Spec.HasSymbol,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("updating password complexity policy: %w", err)
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	cr.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Synced",
			Message:            fmt.Sprintf("PasswordComplexityPolicy synced (minLength=%d)", cr.Spec.MinLength),
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("passwordcomplexitypolicy reconciled", "minLength", cr.Spec.MinLength)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PasswordComplexityPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.PasswordComplexityPolicy{}).
		Named("passwordcomplexitypolicy").
		Complete(r)
}
