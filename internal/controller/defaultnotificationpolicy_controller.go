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
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(&cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultNotificationPolicy") {
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
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
	if r.hasDrift(&cr.Spec, policy) {
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

// hasDrift checks if the current notification policy differs from the desired spec.
func (r *DefaultNotificationPolicyReconciler) hasDrift(spec *zitadelv1alpha2.DefaultNotificationPolicySpec, policy *policyv1.NotificationPolicy) bool {
	if policy == nil {
		return true
	}
	desiredPasswordChange := false
	if spec.PasswordChange != nil {
		desiredPasswordChange = *spec.PasswordChange
	}
	return desiredPasswordChange != policy.GetPasswordChange()
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultNotificationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultNotificationPolicy{}).
		Named("defaultnotificationpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
