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
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// PrivacyPolicyReconciler reconciles a PrivacyPolicy object.
type PrivacyPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
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

	// v0.18 (INF-422/INF-423): dual-serving instance gate + scope
	// resolution. Fail-closed outcomes return immediately; during deletion
	// failures fall back to the binding client so finalizers cannot deadlock.
	ctx, rs, rsDone, rsResult, rsErr := tenantPreamble(ctx, r.Client, r.Config,
		r.Resolver, r.Delegation, r.Zitadel, &cr, cr.Spec.Instance, &cr.Status.Conditions, req.Namespace)
	if rsDone {
		return rsResult, rsErr
	}

	// Resolve organization.
	orgID, err := resolveScopedOrganizationId(ctx, r.Client, rs, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "OrgNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Handle deletion (reset to default; non-blocking).
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		_, resetErr := zclient(ctx, r.Zitadel).Management().ResetPrivacyPolicyToDefault(ctx, &management.ResetPrivacyPolicyToDefaultRequest{})
		if resetErr != nil && status.Code(resetErr) != codes.NotFound {
			return fmt.Errorf("resetting privacy policy to default: %w", resetErr)
		}
		return nil
	}); done {
		return result, err
	}

	// Add finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
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
		if err := applyStatus(ctx, r.Client, r.Config, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("privacypolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *PrivacyPolicyReconciler) ensurePrivacyPolicy(ctx context.Context, cr *zitadelv1alpha2.PrivacyPolicy) error {
	fields := cr.Spec.PrivacyPolicyFields

	// Try to update existing custom privacy policy first.
	_, err := zclient(ctx, r.Zitadel).Management().UpdateCustomPrivacyPolicy(ctx, &management.UpdateCustomPrivacyPolicyRequest{
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
			_, err = zclient(ctx, r.Zitadel).Management().AddCustomPrivacyPolicy(ctx, &management.AddCustomPrivacyPolicyRequest{
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
