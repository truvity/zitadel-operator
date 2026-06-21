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

// PasswordAgePolicyReconciler reconciles a PasswordAgePolicy object.
type PasswordAgePolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordagepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordagepolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordagepolicies/finalizers,verbs=update

func (r *PasswordAgePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.PasswordAgePolicy
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
		_, err := r.Zitadel.Management().ResetPasswordAgePolicyToDefault(ctx, &management.ResetPasswordAgePolicyToDefaultRequest{})
		if err != nil && status.Code(err) != codes.NotFound {
			logger.Info("could not reset password age policy to default", "error", err)
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

	// Ensure password age policy exists.
	_, err = r.Zitadel.Management().UpdateCustomPasswordAgePolicy(ctx, &management.UpdateCustomPasswordAgePolicyRequest{
		MaxAgeDays:     cr.Spec.MaxAgeDays,
		ExpireWarnDays: cr.Spec.ExpireWarnDays,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			_, err = r.Zitadel.Management().AddCustomPasswordAgePolicy(ctx, &management.AddCustomPasswordAgePolicyRequest{
				MaxAgeDays:     cr.Spec.MaxAgeDays,
				ExpireWarnDays: cr.Spec.ExpireWarnDays,
			})
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("adding custom password age policy: %w", err)
			}
		} else {
			return ctrl.Result{}, fmt.Errorf("updating custom password age policy: %w", err)
		}
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

	logger.Info("passwordagepolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PasswordAgePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.PasswordAgePolicy{}).
		Named("passwordagepolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
