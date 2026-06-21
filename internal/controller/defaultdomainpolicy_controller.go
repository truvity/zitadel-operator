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
	policyv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/policy"
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

	// Singleton conflict detection: only the earliest-created CR manages the instance.
	var list zitadelv1alpha2.DefaultDomainPolicyList
	if err := r.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(&cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultDomainPolicy") {
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if shouldResetOnDelete(&cr) {
			// Zitadel documented instance defaults: UserLoginMustBeDomain=true, ValidateOrgDomains=true, SmtpSenderAddressMatchesInstanceDomain=true.
			_, _ = r.Zitadel.Admin().UpdateDomainPolicy(ctx, &admin.UpdateDomainPolicyRequest{
				UserLoginMustBeDomain:                  true,
				ValidateOrgDomains:                     true,
				SmtpSenderAddressMatchesInstanceDomain: true,
			})
			logger.Info("reset instance domain policy to defaults (reset-on-delete annotation present)")
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

	// Read current domain policy from Zitadel.
	current, err := r.Zitadel.Admin().GetDomainPolicy(ctx, &admin.GetDomainPolicyRequest{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting default domain policy: %w", err)
	}

	// Detect drift and update if needed.
	policy := current.GetPolicy()
	if r.hasDrift(&cr.Spec, policy) {
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

// hasDrift checks if the current domain policy differs from the desired spec.
func (r *DefaultDomainPolicyReconciler) hasDrift(spec *zitadelv1alpha2.DefaultDomainPolicySpec, policy *policyv1.DomainPolicy) bool {
	if policy == nil {
		return true
	}
	if spec.UserLoginMustBeDomain != nil && *spec.UserLoginMustBeDomain != policy.GetUserLoginMustBeDomain() {
		return true
	}
	if spec.ValidateOrgDomains != nil && *spec.ValidateOrgDomains != policy.GetValidateOrgDomains() {
		return true
	}
	if spec.SmtpSenderAddressMatchesInstanceDomain != nil && *spec.SmtpSenderAddressMatchesInstanceDomain != policy.GetSmtpSenderAddressMatchesInstanceDomain() {
		return true
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultDomainPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultDomainPolicy{}).
		Named("defaultdomainpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
