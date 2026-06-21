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

	// Handle deletion: reset to safe defaults.
	if !cr.DeletionTimestamp.IsZero() {
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

	// Read current label policy from Zitadel.
	current, err := r.Zitadel.Admin().GetLabelPolicy(ctx, &admin.GetLabelPolicyRequest{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting default label policy: %w", err)
	}

	// Detect drift and update if needed.
	policy := current.GetPolicy()
	drifted := false
	if policy != nil {
		if cr.Spec.PrimaryColor != policy.GetPrimaryColor() {
			drifted = true
		}
		if cr.Spec.BackgroundColor != policy.GetBackgroundColor() {
			drifted = true
		}
		if cr.Spec.WarnColor != policy.GetWarnColor() {
			drifted = true
		}
		if cr.Spec.FontColor != policy.GetFontColor() {
			drifted = true
		}
		if cr.Spec.PrimaryColorDark != policy.GetPrimaryColorDark() {
			drifted = true
		}
		if cr.Spec.BackgroundColorDark != policy.GetBackgroundColorDark() {
			drifted = true
		}
		if cr.Spec.WarnColorDark != policy.GetWarnColorDark() {
			drifted = true
		}
		if cr.Spec.FontColorDark != policy.GetFontColorDark() {
			drifted = true
		}
		if cr.Spec.HideLoginNameSuffix != policy.GetHideLoginNameSuffix() {
			drifted = true
		}
		if cr.Spec.DisableWatermark != policy.GetDisableWatermark() {
			drifted = true
		}
	} else {
		drifted = true
	}

	if drifted {
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
			return ctrl.Result{}, fmt.Errorf("updating default label policy: %w", err)
		}

		// Activate the label policy to make it live.
		_, err = r.Zitadel.Admin().ActivateLabelPolicy(ctx, &admin.ActivateLabelPolicyRequest{})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("activating default label policy: %w", err)
		}

		logger.Info("default label policy updated and activated (drift detected)")
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

	logger.Info("defaultlabelpolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultLabelPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultLabelPolicy{}).
		Named("defaultlabelpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
