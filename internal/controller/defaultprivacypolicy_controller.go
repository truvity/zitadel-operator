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
	for i := range list.Items {
		other := &list.Items[i]
		if other.UID == cr.UID {
			continue
		}
		if other.CreationTimestamp.Before(&cr.CreationTimestamp) && other.DeletionTimestamp.IsZero() {
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "DuplicateSingleton",
				fmt.Sprintf("another DefaultPrivacyPolicy %s/%s (created earlier) is already managing this instance singleton", other.Namespace, other.Name))
			cr.Status.Ready = false
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueInterval}, nil
		}
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
	drifted := false
	if policy != nil {
		if cr.Spec.TosLink != policy.GetTosLink() {
			drifted = true
		}
		if cr.Spec.PrivacyLink != policy.GetPrivacyLink() {
			drifted = true
		}
		if cr.Spec.HelpLink != policy.GetHelpLink() {
			drifted = true
		}
		if cr.Spec.SupportEmail != policy.GetSupportEmail() {
			drifted = true
		}
		if cr.Spec.DocsLink != policy.GetDocsLink() {
			drifted = true
		}
		if cr.Spec.CustomLink != policy.GetCustomLink() {
			drifted = true
		}
		if cr.Spec.CustomLinkText != policy.GetCustomLinkText() {
			drifted = true
		}
	} else {
		drifted = true
	}

	if drifted {
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

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultPrivacyPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultPrivacyPolicy{}).
		Named("defaultprivacypolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
