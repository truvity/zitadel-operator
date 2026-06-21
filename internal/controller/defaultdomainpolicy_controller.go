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

// DefaultDomainPolicyReconciler reconciles a DefaultDomainPolicy object.
type DefaultDomainPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultdomainpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultdomainpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultdomainpolicies/finalizers,verbs=update

func (r *DefaultDomainPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.DefaultDomainPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion: reset to safe defaults.
	if !cr.DeletionTimestamp.IsZero() {
		_, _ = r.Zitadel.Admin().UpdateDomainPolicy(ctx, &admin.UpdateDomainPolicyRequest{
			UserLoginMustBeDomain:                  true,
			ValidateOrgDomains:                     true,
			SmtpSenderAddressMatchesInstanceDomain: true,
		})
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

	// Read current domain policy from Zitadel.
	current, err := r.Zitadel.Admin().GetDomainPolicy(ctx, &admin.GetDomainPolicyRequest{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting default domain policy: %w", err)
	}

	// Detect drift and update if needed.
	policy := current.GetPolicy()
	drifted := false
	if policy != nil {
		if cr.Spec.UserLoginMustBeDomain != nil && *cr.Spec.UserLoginMustBeDomain != policy.GetUserLoginMustBeDomain() {
			drifted = true
		}
		if cr.Spec.ValidateOrgDomains != nil && *cr.Spec.ValidateOrgDomains != policy.GetValidateOrgDomains() {
			drifted = true
		}
		if cr.Spec.SmtpSenderAddressMatchesInstanceDomain != nil && *cr.Spec.SmtpSenderAddressMatchesInstanceDomain != policy.GetSmtpSenderAddressMatchesInstanceDomain() {
			drifted = true
		}
	} else {
		drifted = true
	}

	if drifted {
		_, err := r.Zitadel.Admin().UpdateDomainPolicy(ctx, &admin.UpdateDomainPolicyRequest{
			UserLoginMustBeDomain:                  boolValue(cr.Spec.UserLoginMustBeDomain, true),
			ValidateOrgDomains:                     boolValue(cr.Spec.ValidateOrgDomains, true),
			SmtpSenderAddressMatchesInstanceDomain: boolValue(cr.Spec.SmtpSenderAddressMatchesInstanceDomain, true),
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("updating default domain policy: %w", err)
		}
		logger.Info("default domain policy updated (drift detected)")
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

	logger.Info("defaultdomainpolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultDomainPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultDomainPolicy{}).
		Named("defaultdomainpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
