package controller

import (
	"context"
	"fmt"

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

// PasswordComplexityPolicyReconciler reconciles a PasswordComplexityPolicy object.
type PasswordComplexityPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordcomplexitypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordcomplexitypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=passwordcomplexitypolicies/finalizers,verbs=update

func (r *PasswordComplexityPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.PasswordComplexityPolicy
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

	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		_, err := r.Zitadel.Management().ResetPasswordComplexityPolicyToDefault(ctx, &management.ResetPasswordComplexityPolicyToDefaultRequest{})
		if err != nil && status.Code(err) != codes.NotFound {
			return nil // non-critical
		}
		return nil
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
	statusChanged := cr.Status.OrganizationId != orgID
	cr.Status.OrganizationId = orgID
	if err := markReady(ctx, r.Client, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("passwordcomplexitypolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *PasswordComplexityPolicyReconciler) reconcileSpec(ctx context.Context, cr *zitadelv1alpha2.PasswordComplexityPolicy) error {
	currentResp, err := r.Zitadel.Management().GetPasswordComplexityPolicy(ctx, &management.GetPasswordComplexityPolicyRequest{})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("getting password complexity policy: %w", err)
	}
	return r.syncPolicy(ctx, &cr.Spec, currentResp)
}

// syncPolicy creates or updates the custom password complexity policy if drift is detected.
func (r *PasswordComplexityPolicyReconciler) syncPolicy(ctx context.Context, spec *zitadelv1alpha2.PasswordComplexityPolicySpec, currentResp *management.GetPasswordComplexityPolicyResponse) error {
	needsUpdate := true
	if currentResp != nil && currentResp.GetPolicy() != nil {
		p := currentResp.GetPolicy()
		if p.GetMinLength() == spec.MinLength &&
			p.GetHasLowercase() == spec.HasLowercase &&
			p.GetHasUppercase() == spec.HasUppercase &&
			p.GetHasNumber() == spec.HasNumber &&
			p.GetHasSymbol() == spec.HasSymbol &&
			!p.GetIsDefault() {
			needsUpdate = false
		}
	}

	if !needsUpdate {
		return nil
	}

	// Try update first; if not found (no custom policy), add.
	_, err := r.Zitadel.Management().UpdateCustomPasswordComplexityPolicy(ctx, &management.UpdateCustomPasswordComplexityPolicyRequest{
		MinLength:    spec.MinLength,
		HasLowercase: spec.HasLowercase,
		HasUppercase: spec.HasUppercase,
		HasNumber:    spec.HasNumber,
		HasSymbol:    spec.HasSymbol,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			_, err = r.Zitadel.Management().AddCustomPasswordComplexityPolicy(ctx, &management.AddCustomPasswordComplexityPolicyRequest{
				MinLength:    spec.MinLength,
				HasLowercase: spec.HasLowercase,
				HasUppercase: spec.HasUppercase,
				HasNumber:    spec.HasNumber,
				HasSymbol:    spec.HasSymbol,
			})
			if err != nil {
				return fmt.Errorf("adding custom password complexity policy: %w", err)
			}
		} else {
			return fmt.Errorf("updating custom password complexity policy: %w", err)
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PasswordComplexityPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.PasswordComplexityPolicy{}).
		Named("passwordcomplexitypolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
