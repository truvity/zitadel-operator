package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
)

// DefaultMessageTextReconciler reconciles a DefaultMessageText object.
type DefaultMessageTextReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultmessagetexts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultmessagetexts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultmessagetexts/finalizers,verbs=update

func (r *DefaultMessageTextReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.DefaultMessageText
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Singleton conflict detection (per type+language pair).
	if conflict := r.checkConflict(ctx, &cr); conflict {
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	spec := &cr.Spec.MessageTextFields

	// Deletion.
	if done, result, err := handleSingletonDeletion(ctx, r.Client, &cr, func() {
		if err := r.resetDefaultMessageText(ctx, spec); err != nil {
			logger.Info("could not reset default message text", "type", spec.Type, "error", err)
		} else {
			logger.Info("reset instance message text to defaults (reset-on-delete annotation present)", "type", spec.Type)
		}
	}); done {
		return result, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Business logic.
	r.warnSmsUnusedFields(ctx, &cr, spec)
	if err := r.setDefaultMessageText(ctx, spec); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting default message text (type=%s): %w", spec.Type, err)
	}

	// Status.
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, false); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("defaultmessagetext reconciled", "type", spec.Type, "language", spec.Language)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *DefaultMessageTextReconciler) checkConflict(ctx context.Context, cr *zitadelv1alpha2.DefaultMessageText) bool {
	var list zitadelv1alpha2.DefaultMessageTextList
	if err := r.List(ctx, &list); err != nil {
		return false
	}
	for i := range list.Items {
		other := &list.Items[i]
		if other.UID == cr.UID {
			continue
		}
		if other.Spec.Type == cr.Spec.Type &&
			other.Spec.Language == cr.Spec.Language &&
			other.CreationTimestamp.Before(&cr.CreationTimestamp) && other.DeletionTimestamp.IsZero() {
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "DuplicateSingleton",
				fmt.Sprintf("another DefaultMessageText %s/%s (created earlier) is already managing type=%s language=%s", other.Namespace, other.Name, other.Spec.Type, other.Spec.Language))
			cr.Status.Ready = false
			_ = applyStatus(ctx, r.Client, r.Config, cr)
			return true
		}
	}
	return false
}

func (r *DefaultMessageTextReconciler) warnSmsUnusedFields(ctx context.Context, cr *zitadelv1alpha2.DefaultMessageText, spec *zitadelv1alpha2.MessageTextFields) {
	if spec.Type != "verifySmsOtp" {
		return
	}
	if spec.Title == "" && spec.Subject == "" && spec.PreHeader == "" && spec.Greeting == "" && spec.ButtonText == "" && spec.FooterText == "" {
		return
	}
	logger := log.FromContext(ctx)
	logger.Info("WARNING: verifySmsOtp only uses language and text fields; other fields are ignored by Zitadel")
	setCondition(&cr.Status.Conditions, "SmsFieldWarning", metav1.ConditionTrue, "IgnoredFields", "verifySmsOtp type only uses language and text; other fields are ignored")
}

func (r *DefaultMessageTextReconciler) setDefaultMessageText(ctx context.Context, spec *zitadelv1alpha2.MessageTextFields) error {
	switch spec.Type {
	case "init":
		_, err := r.Zitadel.Admin().SetDefaultInitMessageText(ctx, &admin.SetDefaultInitMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "passwordReset":
		_, err := r.Zitadel.Admin().SetDefaultPasswordResetMessageText(ctx, &admin.SetDefaultPasswordResetMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "verifyEmail":
		_, err := r.Zitadel.Admin().SetDefaultVerifyEmailMessageText(ctx, &admin.SetDefaultVerifyEmailMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "verifyPhone":
		_, err := r.Zitadel.Admin().SetDefaultVerifyPhoneMessageText(ctx, &admin.SetDefaultVerifyPhoneMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "verifySmsOtp":
		_, err := r.Zitadel.Admin().SetDefaultVerifySMSOTPMessageText(ctx, &admin.SetDefaultVerifySMSOTPMessageTextRequest{
			Language: spec.Language, Text: spec.Text,
		})
		return err
	case "verifyEmailOtp":
		_, err := r.Zitadel.Admin().SetDefaultVerifyEmailOTPMessageText(ctx, &admin.SetDefaultVerifyEmailOTPMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "domainClaimed":
		_, err := r.Zitadel.Admin().SetDefaultDomainClaimedMessageText(ctx, &admin.SetDefaultDomainClaimedMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "passwordlessRegistration":
		_, err := r.Zitadel.Admin().SetDefaultPasswordlessRegistrationMessageText(ctx, &admin.SetDefaultPasswordlessRegistrationMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "passwordChange":
		_, err := r.Zitadel.Admin().SetDefaultPasswordChangeMessageText(ctx, &admin.SetDefaultPasswordChangeMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "inviteUser":
		_, err := r.Zitadel.Admin().SetDefaultInviteUserMessageText(ctx, &admin.SetDefaultInviteUserMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	default:
		return fmt.Errorf("unsupported message text type: %s", spec.Type)
	}
}

func (r *DefaultMessageTextReconciler) resetDefaultMessageText(ctx context.Context, spec *zitadelv1alpha2.MessageTextFields) error {
	switch spec.Type {
	case "init":
		_, err := r.Zitadel.Admin().ResetCustomInitMessageTextToDefault(ctx, &admin.ResetCustomInitMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "passwordReset":
		_, err := r.Zitadel.Admin().ResetCustomPasswordResetMessageTextToDefault(ctx, &admin.ResetCustomPasswordResetMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifyEmail":
		_, err := r.Zitadel.Admin().ResetCustomVerifyEmailMessageTextToDefault(ctx, &admin.ResetCustomVerifyEmailMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifyPhone":
		_, err := r.Zitadel.Admin().ResetCustomVerifyPhoneMessageTextToDefault(ctx, &admin.ResetCustomVerifyPhoneMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifySmsOtp":
		_, err := r.Zitadel.Admin().ResetCustomVerifySMSOTPMessageTextToDefault(ctx, &admin.ResetCustomVerifySMSOTPMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifyEmailOtp":
		_, err := r.Zitadel.Admin().ResetCustomVerifyEmailOTPMessageTextToDefault(ctx, &admin.ResetCustomVerifyEmailOTPMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "domainClaimed":
		_, err := r.Zitadel.Admin().ResetCustomDomainClaimedMessageTextToDefault(ctx, &admin.ResetCustomDomainClaimedMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "passwordlessRegistration":
		_, err := r.Zitadel.Admin().ResetCustomPasswordlessRegistrationMessageTextToDefault(ctx, &admin.ResetCustomPasswordlessRegistrationMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "passwordChange":
		_, err := r.Zitadel.Admin().ResetCustomPasswordChangeMessageTextToDefault(ctx, &admin.ResetCustomPasswordChangeMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "inviteUser":
		_, err := r.Zitadel.Admin().ResetCustomInviteUserMessageTextToDefault(ctx, &admin.ResetCustomInviteUserMessageTextToDefaultRequest{Language: spec.Language})
		return err
	default:
		return fmt.Errorf("unsupported message text type: %s", spec.Type)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultMessageTextReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultMessageText{}).
		Named("defaultmessagetext").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
