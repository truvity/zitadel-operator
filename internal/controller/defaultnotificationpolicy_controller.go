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

	// Singleton conflict detection: only the earliest-created CR manages the instance.
	var list zitadelv1alpha2.DefaultNotificationPolicyList
	if err := r.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}
	for i := range list.Items {
		other := &list.Items[i]
		if other.UID == cr.UID {
			continue
		}
		if other.CreationTimestamp.Before(&cr.CreationTimestamp) && other.DeletionTimestamp.IsZero() {
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "DuplicateSingleton",
				fmt.Sprintf("another DefaultNotificationPolicy %s/%s (created earlier) is already managing this instance singleton", other.Namespace, other.Name))
			cr.Status.Ready = false
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueInterval}, nil
		}
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if shouldResetOnDelete(&cr) {
			// Zitadel documented instance defaults: no notification on password change.
			_, _ = r.Zitadel.Admin().UpdateNotificationPolicy(ctx, &admin.UpdateNotificationPolicyRequest{
				PasswordChange: false,
			})
			logger.Info("reset instance notification policy to defaults (reset-on-delete annotation present)")
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

	// Read current notification policy from Zitadel.
	current, err := r.Zitadel.Admin().GetNotificationPolicy(ctx, &admin.GetNotificationPolicyRequest{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting default notification policy: %w", err)
	}

	// Detect drift and update if needed.
	policy := current.GetPolicy()
	drifted := false
	if policy != nil {
		desiredPasswordChange := false
		if cr.Spec.PasswordChange != nil {
			desiredPasswordChange = *cr.Spec.PasswordChange
		}
		if desiredPasswordChange != policy.GetPasswordChange() {
			drifted = true
		}
	} else {
		drifted = true
	}

	if drifted {
		passwordChange := false
		if cr.Spec.PasswordChange != nil {
			passwordChange = *cr.Spec.PasswordChange
		}
		_, err := r.Zitadel.Admin().UpdateNotificationPolicy(ctx, &admin.UpdateNotificationPolicyRequest{
			PasswordChange: passwordChange,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("updating default notification policy: %w", err)
		}
		logger.Info("default notification policy updated (drift detected)")
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

	logger.Info("defaultnotificationpolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultNotificationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultNotificationPolicy{}).
		Named("defaultnotificationpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
