package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/metadata"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// MessageTextReconciler reconciles a MessageText object.
type MessageTextReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=messagetexts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=messagetexts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=messagetexts/finalizers,verbs=update

func (r *MessageTextReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.MessageText
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
	spec := &cr.Spec.MessageTextFields

	// Handle deletion: reset to default.
	if !cr.DeletionTimestamp.IsZero() {
		if err := r.resetCustomMessageText(ctx, spec); err != nil {
			logger.Info("could not reset custom message text", "type", spec.Type, "error", err)
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

	// Warn if email-only fields are set for verifySmsOtp.
	if spec.Type == "verifySmsOtp" {
		if spec.Title != "" || spec.Subject != "" || spec.PreHeader != "" || spec.Greeting != "" || spec.ButtonText != "" || spec.FooterText != "" {
			logger.Info("WARNING: verifySmsOtp only uses language and text fields; other fields are ignored by Zitadel")
			setCondition(&cr.Status.Conditions, "SmsFieldWarning", metav1.ConditionTrue, "IgnoredFields", "verifySmsOtp type only uses language and text; other fields are ignored")
		}
	}

	// Set the custom message text (idempotent — always call Set on every reconcile).
	if err := r.setCustomMessageText(ctx, spec); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting custom message text (type=%s): %w", spec.Type, err)
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

	logger.Info("messagetext reconciled", "type", spec.Type, "language", spec.Language, "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *MessageTextReconciler) setCustomMessageText(ctx context.Context, spec *zitadelv1alpha2.MessageTextFields) error {
	switch spec.Type {
	case "init":
		_, err := r.Zitadel.Management().SetCustomInitMessageText(ctx, &management.SetCustomInitMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "passwordReset":
		_, err := r.Zitadel.Management().SetCustomPasswordResetMessageText(ctx, &management.SetCustomPasswordResetMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "verifyEmail":
		_, err := r.Zitadel.Management().SetCustomVerifyEmailMessageText(ctx, &management.SetCustomVerifyEmailMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "verifyPhone":
		_, err := r.Zitadel.Management().SetCustomVerifyPhoneMessageText(ctx, &management.SetCustomVerifyPhoneMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "verifySmsOtp":
		_, err := r.Zitadel.Management().SetCustomVerifySMSOTPMessageText(ctx, &management.SetCustomVerifySMSOTPMessageTextRequest{
			Language: spec.Language, Text: spec.Text,
		})
		return err
	case "verifyEmailOtp":
		_, err := r.Zitadel.Management().SetCustomVerifyEmailOTPMessageText(ctx, &management.SetCustomVerifyEmailOTPMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "domainClaimed":
		_, err := r.Zitadel.Management().SetCustomDomainClaimedMessageCustomText(ctx, &management.SetCustomDomainClaimedMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "passwordlessRegistration":
		_, err := r.Zitadel.Management().SetCustomPasswordlessRegistrationMessageCustomText(ctx, &management.SetCustomPasswordlessRegistrationMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "passwordChange":
		_, err := r.Zitadel.Management().SetCustomPasswordChangeMessageCustomText(ctx, &management.SetCustomPasswordChangeMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "inviteUser":
		_, err := r.Zitadel.Management().SetCustomInviteUserMessageCustomText(ctx, &management.SetCustomInviteUserMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	default:
		return fmt.Errorf("unsupported message text type: %s", spec.Type)
	}
}

func (r *MessageTextReconciler) resetCustomMessageText(ctx context.Context, spec *zitadelv1alpha2.MessageTextFields) error {
	switch spec.Type {
	case "init":
		_, err := r.Zitadel.Management().ResetCustomInitMessageTextToDefault(ctx, &management.ResetCustomInitMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "passwordReset":
		_, err := r.Zitadel.Management().ResetCustomPasswordResetMessageTextToDefault(ctx, &management.ResetCustomPasswordResetMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifyEmail":
		_, err := r.Zitadel.Management().ResetCustomVerifyEmailMessageTextToDefault(ctx, &management.ResetCustomVerifyEmailMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifyPhone":
		_, err := r.Zitadel.Management().ResetCustomVerifyPhoneMessageTextToDefault(ctx, &management.ResetCustomVerifyPhoneMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifySmsOtp":
		_, err := r.Zitadel.Management().ResetCustomVerifySMSOTPMessageTextToDefault(ctx, &management.ResetCustomVerifySMSOTPMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifyEmailOtp":
		_, err := r.Zitadel.Management().ResetCustomVerifyEmailOTPMessageTextToDefault(ctx, &management.ResetCustomVerifyEmailOTPMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "domainClaimed":
		_, err := r.Zitadel.Management().ResetCustomDomainClaimedMessageTextToDefault(ctx, &management.ResetCustomDomainClaimedMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "passwordlessRegistration":
		_, err := r.Zitadel.Management().ResetCustomPasswordlessRegistrationMessageTextToDefault(ctx, &management.ResetCustomPasswordlessRegistrationMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "passwordChange":
		_, err := r.Zitadel.Management().ResetCustomPasswordChangeMessageTextToDefault(ctx, &management.ResetCustomPasswordChangeMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "inviteUser":
		_, err := r.Zitadel.Management().ResetCustomInviteUserMessageTextToDefault(ctx, &management.ResetCustomInviteUserMessageTextToDefaultRequest{Language: spec.Language})
		return err
	default:
		return fmt.Errorf("unsupported message text type: %s", spec.Type)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MessageTextReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.MessageText{}).
		Named("messagetext").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
