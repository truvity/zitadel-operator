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

// PrivacyPolicyReconciler reconciles a PrivacyPolicy object.
type PrivacyPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=privacypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=privacypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=privacypolicies/finalizers,verbs=update

func (r *PrivacyPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.PrivacyPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve organization.
	orgID, err := resolveOrganizationId(ctx, r.Client, r.Config, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for organization ref to become ready", "error", err)
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "OrgNotReady", err.Error())
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		_, err := r.Zitadel.Management().ResetPrivacyPolicyToDefault(ctx, &management.ResetPrivacyPolicyToDefaultRequest{})
		if err != nil && status.Code(err) != codes.NotFound {
			logger.Info("could not reset privacy policy to default", "error", err)
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

	// Ensure privacy policy exists.
	err = r.ensurePrivacyPolicy(ctx, &cr)
	if err != nil {
		return ctrl.Result{}, err
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

	logger.Info("privacypolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *PrivacyPolicyReconciler) ensurePrivacyPolicy(ctx context.Context, cr *zitadelv1alpha2.PrivacyPolicy) error {
	fields := cr.Spec.PrivacyPolicyFields

	// Try to update existing custom privacy policy first.
	_, err := r.Zitadel.Management().UpdateCustomPrivacyPolicy(ctx, &management.UpdateCustomPrivacyPolicyRequest{
		TosLink:        fields.TosLink,
		PrivacyLink:    fields.PrivacyLink,
		HelpLink:       fields.HelpLink,
		SupportEmail:   fields.SupportEmail,
		DocsLink:       fields.DocsLink,
		CustomLink:     fields.CustomLink,
		CustomLinkText: fields.CustomLinkText,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// No custom policy exists, create one.
			_, err = r.Zitadel.Management().AddCustomPrivacyPolicy(ctx, &management.AddCustomPrivacyPolicyRequest{
				TosLink:        fields.TosLink,
				PrivacyLink:    fields.PrivacyLink,
				HelpLink:       fields.HelpLink,
				SupportEmail:   fields.SupportEmail,
				DocsLink:       fields.DocsLink,
				CustomLink:     fields.CustomLink,
				CustomLinkText: fields.CustomLinkText,
			})
			if err != nil {
				return fmt.Errorf("adding custom privacy policy: %w", err)
			}
		} else {
			return fmt.Errorf("updating custom privacy policy: %w", err)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PrivacyPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.PrivacyPolicy{}).
		Named("privacypolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
