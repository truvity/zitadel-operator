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
)

// ApplicationKeyReconciler reconciles an ApplicationKey object.
type ApplicationKeyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=applicationkeys,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=applicationkeys/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=applicationkeys/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func (r *ApplicationKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha1.ApplicationKey
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		// TODO: Delete application key from Zitadel.
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

	// TODO: Implement application key creation/sync with Zitadel API.
	logger.Info("applicationkey reconcile not yet implemented",
		"appId", cr.Spec.AppId,
		"projectRef", cr.Spec.ProjectRef)

	// Update status as not ready (not yet implemented).
	now := metav1.NewTime(time.Now())
	cr.Status.Ready = false
	cr.Status.LastSyncTime = &now
	cr.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "NotImplemented",
			Message:            fmt.Sprintf("ApplicationKey reconciliation not yet implemented for app %q", cr.Spec.AppId),
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.ApplicationKey{}).
		Named("applicationkey").
		WithEventFilter(generationChangedPredicate()).
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
