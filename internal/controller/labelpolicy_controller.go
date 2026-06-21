package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// LabelPolicyReconciler reconciles a LabelPolicy object.
type LabelPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=labelpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=labelpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=labelpolicies/finalizers,verbs=update

func (r *LabelPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.LabelPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve organization.
	orgID, err := resolveOrganizationId(ctx, r.Client, r.Config, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for organization ref to become ready", "error", err)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		_, err := r.Zitadel.Management().ResetLabelPolicyToDefault(ctx, &management.ResetLabelPolicyToDefaultRequest{})
		if err != nil && status.Code(err) != codes.NotFound {
			logger.Info("could not reset label policy to default", "error", err)
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

	// Ensure label policy exists.
	err = r.ensureLabelPolicy(ctx, &cr)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Activate the label policy to make it live.
	_, err = r.Zitadel.Management().ActivateCustomLabelPolicy(ctx, &management.ActivateCustomLabelPolicyRequest{})
	if err != nil && status.Code(err) != codes.FailedPrecondition {
		logger.Info("could not activate label policy (may already be active)", "error", err)
	}

	// Update status.
	if cr.Status.OrganizationId != orgID || !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.OrganizationId = orgID
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("labelpolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *LabelPolicyReconciler) ensureLabelPolicy(ctx context.Context, cr *zitadelv1alpha2.LabelPolicy) error {
	fields := cr.Spec.LabelPolicyFields

	// Try to update existing custom label policy first.
	_, err := r.Zitadel.Management().UpdateCustomLabelPolicy(ctx, &management.UpdateCustomLabelPolicyRequest{
		PrimaryColor:        fields.PrimaryColor,
		BackgroundColor:     fields.BackgroundColor,
		WarnColor:           fields.WarnColor,
		FontColor:           fields.FontColor,
		PrimaryColorDark:    fields.PrimaryColorDark,
		BackgroundColorDark: fields.BackgroundColorDark,
		WarnColorDark:       fields.WarnColorDark,
		FontColorDark:       fields.FontColorDark,
		HideLoginNameSuffix: fields.HideLoginNameSuffix,
		DisableWatermark:    fields.DisableWatermark,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// No custom policy exists, create one.
			_, err = r.Zitadel.Management().AddCustomLabelPolicy(ctx, &management.AddCustomLabelPolicyRequest{
				PrimaryColor:        fields.PrimaryColor,
				BackgroundColor:     fields.BackgroundColor,
				WarnColor:           fields.WarnColor,
				FontColor:           fields.FontColor,
				PrimaryColorDark:    fields.PrimaryColorDark,
				BackgroundColorDark: fields.BackgroundColorDark,
				WarnColorDark:       fields.WarnColorDark,
				FontColorDark:       fields.FontColorDark,
				HideLoginNameSuffix: fields.HideLoginNameSuffix,
				DisableWatermark:    fields.DisableWatermark,
			})
			if err != nil {
				return fmt.Errorf("adding custom label policy: %w", err)
			}
		} else {
			return fmt.Errorf("updating custom label policy: %w", err)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LabelPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.LabelPolicy{}).
		Named("labelpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
