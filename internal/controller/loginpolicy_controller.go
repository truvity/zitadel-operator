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

// LoginPolicyReconciler reconciles a LoginPolicy object.
type LoginPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=loginpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=loginpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=loginpolicies/finalizers,verbs=update

func (r *LoginPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the LoginPolicy CR.
	var cr zitadelv1alpha1.LoginPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion — no cleanup needed, just remove finalizer.
	if !cr.DeletionTimestamp.IsZero() {
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

	// Update login policy settings via Admin service.
	_, err := r.Zitadel.Admin().UpdateLoginPolicy(ctx, &admin.UpdateLoginPolicyRequest{
		AllowUsernamePassword: true,
		AllowRegister:         cr.Spec.AllowRegister,
		AllowExternalIdp:      cr.Spec.AllowExternalIdp,
		HidePasswordReset:     cr.Spec.HidePasswordReset,
		DisableLoginWithEmail: cr.Spec.DisableLoginWithEmail,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("updating login policy: %w", err)
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("loginpolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoginPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.LoginPolicy{}).
		Named("loginpolicy").
		Complete(r)
}
