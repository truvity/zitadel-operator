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

	// Singleton conflict detection: only the earliest-created CR manages the instance.
	var list zitadelv1alpha2.DefaultPasswordAgePolicyList
	if err := r.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(&cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultPasswordAgePolicy") {
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if shouldResetOnDelete(&cr) {
			// Zitadel documented instance defaults: no password expiry (MaxAgeDays=0, ExpireWarnDays=0).
			_, _ = r.Zitadel.Admin().UpdatePasswordAgePolicy(ctx, &admin.UpdatePasswordAgePolicyRequest{
				MaxAgeDays:     0,
				ExpireWarnDays: 0,
			})
			logger.Info("reset instance password age policy to defaults (reset-on-delete annotation present)")
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

	// Read current password age policy from Zitadel.
	current, err := r.Zitadel.Admin().GetPasswordAgePolicy(ctx, &admin.GetPasswordAgePolicyRequest{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting default password age policy: %w", err)
	}

	// Detect drift and update if needed.
	policy := current.GetPolicy()
	if r.hasDrift(&cr.Spec, policy) {
		_, err := r.Zitadel.Admin().UpdatePasswordAgePolicy(ctx, &admin.UpdatePasswordAgePolicyRequest{
			MaxAgeDays:     cr.Spec.MaxAgeDays,
			ExpireWarnDays: cr.Spec.ExpireWarnDays,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("updating default password age policy: %w", err)
		}
		logger.Info("default password age policy updated (drift detected)")
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

	logger.Info("defaultpasswordagepolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
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
