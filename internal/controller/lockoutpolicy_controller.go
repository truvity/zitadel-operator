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
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Handle deletion: reset to default.
	if !cr.DeletionTimestamp.IsZero() {
		_, err := r.Zitadel.Management().ResetLockoutPolicyToDefault(ctx, &management.ResetLockoutPolicyToDefaultRequest{})
		if err != nil && status.Code(err) != codes.NotFound {
			logger.Info("could not reset lockout policy to default", "error", err)
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

	// Check current lockout policy.
	currentResp, err := r.Zitadel.Management().GetLockoutPolicy(ctx, &management.GetLockoutPolicyRequest{})
	if err != nil && status.Code(err) != codes.NotFound {
		return ctrl.Result{}, fmt.Errorf("getting lockout policy: %w", err)
	}

	// Sync the policy.
	if err := r.syncPolicy(ctx, &cr.Spec, currentResp); err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	statusChanged := cr.Status.OrganizationId != orgID || !cr.Status.Ready
	if statusChanged {
		now := metav1.NewTime(time.Now())
		cr.Status.OrganizationId = orgID
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("lockoutpolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// syncPolicy creates or updates the custom lockout policy if drift is detected.
func (r *LockoutPolicyReconciler) syncPolicy(ctx context.Context, spec *zitadelv1alpha2.LockoutPolicySpec, currentResp *management.GetLockoutPolicyResponse) error {
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

	_, err := r.Zitadel.Management().UpdateCustomLockoutPolicy(ctx, &management.UpdateCustomLockoutPolicyRequest{
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
