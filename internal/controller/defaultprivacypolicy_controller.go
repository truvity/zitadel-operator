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

// DefaultPrivacyPolicyReconciler reconciles a DefaultPrivacyPolicy object.
type DefaultPrivacyPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultprivacypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultprivacypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultprivacypolicies/finalizers,verbs=update

func (r *DefaultPrivacyPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.DefaultPrivacyPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Singleton conflict detection: only the earliest-created CR manages the instance.
	var list zitadelv1alpha2.DefaultPrivacyPolicyList
	if err := r.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(&cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultPrivacyPolicy") {
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if shouldResetOnDelete(&cr) {
			// Zitadel documented instance defaults: no links (all empty strings).
			_, _ = r.Zitadel.Admin().UpdatePrivacyPolicy(ctx, &admin.UpdatePrivacyPolicyRequest{
				TosLink:        "",
				PrivacyLink:    "",
				HelpLink:       "",
				SupportEmail:   "",
				DocsLink:       "",
				CustomLink:     "",
				CustomLinkText: "",
			})
			logger.Info("reset instance privacy policy to defaults (reset-on-delete annotation present)")
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

	// Read current privacy policy from Zitadel.
	current, err := r.Zitadel.Admin().GetPrivacyPolicy(ctx, &admin.GetPrivacyPolicyRequest{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting default privacy policy: %w", err)
	}

	// Detect drift and update if needed.
	policy := current.GetPolicy()
	if r.hasDrift(&cr.Spec, policy) {
		_, err := r.Zitadel.Admin().UpdatePrivacyPolicy(ctx, &admin.UpdatePrivacyPolicyRequest{
			TosLink:        cr.Spec.TosLink,
			PrivacyLink:    cr.Spec.PrivacyLink,
			HelpLink:       cr.Spec.HelpLink,
			SupportEmail:   cr.Spec.SupportEmail,
			DocsLink:       cr.Spec.DocsLink,
			CustomLink:     cr.Spec.CustomLink,
			CustomLinkText: cr.Spec.CustomLinkText,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("updating default privacy policy: %w", err)
		}
		logger.Info("default privacy policy updated (drift detected)")
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

	logger.Info("defaultprivacypolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// hasDrift checks if the current privacy policy differs from the desired spec.
func (r *DefaultPrivacyPolicyReconciler) hasDrift(spec *zitadelv1alpha2.DefaultPrivacyPolicySpec, policy *policyv1.PrivacyPolicy) bool {
	if policy == nil {
		return true
	}
	if spec.TosLink != policy.GetTosLink() {
		return true
	}
	if spec.PrivacyLink != policy.GetPrivacyLink() {
		return true
	}
	if spec.HelpLink != policy.GetHelpLink() {
		return true
	}
	if spec.SupportEmail != policy.GetSupportEmail() {
		return true
	}
	if spec.DocsLink != policy.GetDocsLink() {
		return true
	}
	if spec.CustomLink != policy.GetCustomLink() {
		return true
	}
	if spec.CustomLinkText != policy.GetCustomLinkText() {
		return true
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultPrivacyPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultPrivacyPolicy{}).
		Named("defaultprivacypolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
