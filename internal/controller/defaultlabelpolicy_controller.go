package controller

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	policyv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/policy"
)

// DefaultLabelPolicyReconciler reconciles a DefaultLabelPolicy object.
type DefaultLabelPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultlabelpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultlabelpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultlabelpolicies/finalizers,verbs=update

func (r *DefaultLabelPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.DefaultLabelPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Singleton conflict detection.
	if conflict, err := r.checkConflict(ctx, &cr); err != nil || conflict {
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	}

	// Deletion.
	if done, result, err := handleSingletonDeletion(ctx, r.Client, &cr, func() {
		_, _ = r.Zitadel.Admin().UpdateLabelPolicy(ctx, &admin.UpdateLabelPolicyRequest{
			PrimaryColor:        "",
			BackgroundColor:     "",
			WarnColor:           "",
			FontColor:           "",
			PrimaryColorDark:    "",
			BackgroundColorDark: "",
			WarnColorDark:       "",
			FontColorDark:       "",
			HideLoginNameSuffix: false,
			DisableWatermark:    false,
		})
		_, _ = r.Zitadel.Admin().ActivateLabelPolicy(ctx, &admin.ActivateLabelPolicyRequest{})
		logger.Info("reset instance label policy to defaults (reset-on-delete annotation present)")
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
	if err := markReady(ctx, r.Client, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, false); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("defaultlabelpolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *DefaultLabelPolicyReconciler) checkConflict(ctx context.Context, cr *zitadelv1alpha2.DefaultLabelPolicy) (bool, error) {
	var list zitadelv1alpha2.DefaultLabelPolicyList
	if err := r.List(ctx, &list); err != nil {
		return false, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultLabelPolicy") {
		_ = r.Status().Update(ctx, cr)
		return true, nil
	}
	return false, nil
}

func (r *DefaultLabelPolicyReconciler) reconcileSpec(ctx context.Context, cr *zitadelv1alpha2.DefaultLabelPolicy) error {
	logger := log.FromContext(ctx)
	current, err := r.Zitadel.Admin().GetLabelPolicy(ctx, &admin.GetLabelPolicyRequest{})
	if err != nil {
		return fmt.Errorf("getting default label policy: %w", err)
	}
	if r.hasDrift(&cr.Spec, current.GetPolicy()) {
		_, err := r.Zitadel.Admin().UpdateLabelPolicy(ctx, &admin.UpdateLabelPolicyRequest{
			PrimaryColor:        cr.Spec.PrimaryColor,
			BackgroundColor:     cr.Spec.BackgroundColor,
			WarnColor:           cr.Spec.WarnColor,
			FontColor:           cr.Spec.FontColor,
			PrimaryColorDark:    cr.Spec.PrimaryColorDark,
			BackgroundColorDark: cr.Spec.BackgroundColorDark,
			WarnColorDark:       cr.Spec.WarnColorDark,
			FontColorDark:       cr.Spec.FontColorDark,
			HideLoginNameSuffix: cr.Spec.HideLoginNameSuffix,
			DisableWatermark:    cr.Spec.DisableWatermark,
		})
		if err != nil {
			return fmt.Errorf("updating default label policy: %w", err)
		}
		_, err = r.Zitadel.Admin().ActivateLabelPolicy(ctx, &admin.ActivateLabelPolicyRequest{})
		if err != nil {
			return fmt.Errorf("activating default label policy: %w", err)
		}
		logger.Info("default label policy updated and activated (drift detected)")
	}
	return nil
}

// hasDrift checks if the current label policy differs from the desired spec.
func (r *DefaultLabelPolicyReconciler) hasDrift(spec *zitadelv1alpha2.DefaultLabelPolicySpec, policy *policyv1.LabelPolicy) bool {
	if policy == nil {
		return true
	}
	if spec.PrimaryColor != policy.GetPrimaryColor() {
		return true
	}
	if spec.BackgroundColor != policy.GetBackgroundColor() {
		return true
	}
	if spec.WarnColor != policy.GetWarnColor() {
		return true
	}
	if spec.FontColor != policy.GetFontColor() {
		return true
	}
	if spec.PrimaryColorDark != policy.GetPrimaryColorDark() {
		return true
	}
	if spec.BackgroundColorDark != policy.GetBackgroundColorDark() {
		return true
	}
	if spec.WarnColorDark != policy.GetWarnColorDark() {
		return true
	}
	if spec.FontColorDark != policy.GetFontColorDark() {
		return true
	}
	if spec.HideLoginNameSuffix != policy.GetHideLoginNameSuffix() {
		return true
	}
	if spec.DisableWatermark != policy.GetDisableWatermark() {
		return true
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultLabelPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultLabelPolicy{}).
		Named("defaultlabelpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
