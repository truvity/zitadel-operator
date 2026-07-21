package controller

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	policyv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/policy"
)

// DefaultPrivacyPolicyReconciler reconciles a DefaultPrivacyPolicy object.
type DefaultPrivacyPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
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

	// INF-424 degradation matrix: instance-level resources are not supported
	// under an org-owner binding (no-op during deletion so finalizers complete).
	if done, result, err := checkBindingLevel(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, &cr.Status.Ready); done {
		return result, err
	}

	// Singleton conflict detection.
	if conflict, err := r.checkConflict(ctx, &cr); err != nil || conflict {
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	}

	// Deletion.
	if done, result, err := handleSingletonDeletion(ctx, r.Client, &cr, func() {
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
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, false); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("defaultprivacypolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *DefaultPrivacyPolicyReconciler) checkConflict(ctx context.Context, cr *zitadelv1alpha2.DefaultPrivacyPolicy) (bool, error) {
	var list zitadelv1alpha2.DefaultPrivacyPolicyList
	if err := r.List(ctx, &list); err != nil {
		return false, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultPrivacyPolicy") {
		_ = applyStatus(ctx, r.Client, r.Config, cr)
		return true, nil
	}
	return false, nil
}

func (r *DefaultPrivacyPolicyReconciler) reconcileSpec(ctx context.Context, cr *zitadelv1alpha2.DefaultPrivacyPolicy) error {
	logger := log.FromContext(ctx)
	current, err := r.Zitadel.Admin().GetPrivacyPolicy(ctx, &admin.GetPrivacyPolicyRequest{})
	if err != nil {
		return fmt.Errorf("getting default privacy policy: %w", err)
	}
	if r.hasDrift(&cr.Spec, current.GetPolicy()) {
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
			return fmt.Errorf("updating default privacy policy: %w", err)
		}
		logger.Info("default privacy policy updated (drift detected)")
	}
	return nil
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
