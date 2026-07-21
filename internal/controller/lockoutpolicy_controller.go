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

// LockoutPolicyReconciler reconciles a LockoutPolicy object.
type LockoutPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=lockoutpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=lockoutpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=lockoutpolicies/finalizers,verbs=update

func (r *LockoutPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.LockoutPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve organization.
	orgID, err := resolveOrganizationId(ctx, r.Client, r.Config, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for organization ref to become ready", "error", err)
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "OrgNotReady", err.Error())
			_ = applyStatus(ctx, r.Client, r.Config, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		_, err := r.Zitadel.Management().ResetLockoutPolicyToDefault(ctx, &management.ResetLockoutPolicyToDefaultRequest{})
		if err != nil && status.Code(err) != codes.NotFound {
			return err
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
	if err := r.syncPolicy(ctx, &cr.Spec); err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := cr.Status.OrganizationId != orgID
	cr.Status.OrganizationId = orgID
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("lockoutpolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// syncPolicy creates or updates the custom lockout policy if drift is detected.
func (r *LockoutPolicyReconciler) syncPolicy(ctx context.Context, spec *zitadelv1alpha2.LockoutPolicySpec) error {
	currentResp, err := r.Zitadel.Management().GetLockoutPolicy(ctx, &management.GetLockoutPolicyRequest{})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("getting lockout policy: %w", err)
	}

	needsUpdate := true
	if currentResp != nil && currentResp.GetPolicy() != nil {
		p := currentResp.GetPolicy()
		if p.GetMaxPasswordAttempts() == uint64(spec.MaxPasswordAttempts) &&
			p.GetMaxOtpAttempts() == uint64(spec.MaxOtpAttempts) &&
			!p.GetIsDefault() {
			needsUpdate = false
		}
	}

	if !needsUpdate {
		return nil
	}

	_, err = r.Zitadel.Management().UpdateCustomLockoutPolicy(ctx, &management.UpdateCustomLockoutPolicyRequest{
		MaxPasswordAttempts: spec.MaxPasswordAttempts,
		MaxOtpAttempts:      spec.MaxOtpAttempts,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			_, err = r.Zitadel.Management().AddCustomLockoutPolicy(ctx, &management.AddCustomLockoutPolicyRequest{
				MaxPasswordAttempts: spec.MaxPasswordAttempts,
				MaxOtpAttempts:      spec.MaxOtpAttempts,
			})
			if err != nil {
				return fmt.Errorf("adding custom lockout policy: %w", err)
			}
		} else {
			return fmt.Errorf("updating custom lockout policy: %w", err)
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LockoutPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.LockoutPolicy{}).
		Named("lockoutpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
