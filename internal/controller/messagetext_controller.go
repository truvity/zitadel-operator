package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"

	"google.golang.org/grpc/metadata"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"

	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"
)

// MessageTextReconciler reconciles a MessageText object.
type MessageTextReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=messagetexts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=messagetexts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=messagetexts/finalizers,verbs=update

func (r *MessageTextReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cr zitadelv1alpha2.MessageText
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// v0.18 (INF-422/INF-423): dual-serving instance gate + scope
	// resolution. Fail-closed outcomes return immediately; during deletion
	// failures fall back to the binding client so finalizers cannot deadlock.
	ctx, rs, rsDone, rsResult, rsErr := tenantPreamble(ctx, r.Client, r.Config,
		r.Resolver, r.Delegation, r.Zitadel, &cr, cr.Spec.Instance, &cr.Status.Conditions, req.Namespace)
	if rsDone {
		return rsResult, rsErr
	}

	// Resolve organization.
	orgID, err := resolveScopedOrganizationId(ctx, r.Client, rs, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "OrgNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)
	spec := &cr.Spec.MessageTextFields

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		if resetErr := r.resetCustomMessageText(ctx, spec); resetErr != nil {
			log.FromContext(ctx).Info("could not reset custom message text", "type", spec.Type, "error", resetErr)
		}
		return nil
	}); done {
		return result, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}
	// ensureFinalizer's full-object Update refreshed the object from the
	// server, dropping in-memory condition edits — re-apply ScopeResolved.
	applyScopeResolvedCondition(rs, &cr.Status.Conditions)

	// Warn if email-only fields are set for verifySmsOtp.
	r.warnSmsOtpFields(ctx, &cr, spec)

	// Set the custom message text (idempotent — always call Set on every reconcile).
	if err := r.setCustomMessageText(ctx, spec); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting custom message text (type=%s): %w", spec.Type, err)
	}

	// Status.
	statusChanged := cr.Status.OrganizationId != orgID
	cr.Status.OrganizationId = orgID
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *MessageTextReconciler) warnSmsOtpFields(ctx context.Context, cr *zitadelv1alpha2.MessageText, spec *zitadelv1alpha2.MessageTextFields) {
	if spec.Type != "verifySmsOtp" {
		return
	}
	if spec.Title != "" || spec.Subject != "" || spec.PreHeader != "" || spec.Greeting != "" || spec.ButtonText != "" || spec.FooterText != "" {
		log.FromContext(ctx).Info("WARNING: verifySmsOtp only uses language and text fields; other fields are ignored by Zitadel")
		setCondition(&cr.Status.Conditions, "SmsFieldWarning", metav1.ConditionTrue, "IgnoredFields", "verifySmsOtp type only uses language and text; other fields are ignored")
	}
}

func (r *MessageTextReconciler) setCustomMessageText(ctx context.Context, spec *zitadelv1alpha2.MessageTextFields) error {
	switch spec.Type {
	case "init":
		_, err := zclient(ctx, r.Zitadel).Management().SetCustomInitMessageText(ctx, &management.SetCustomInitMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "passwordReset":
		_, err := zclient(ctx, r.Zitadel).Management().SetCustomPasswordResetMessageText(ctx, &management.SetCustomPasswordResetMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "verifyEmail":
		_, err := zclient(ctx, r.Zitadel).Management().SetCustomVerifyEmailMessageText(ctx, &management.SetCustomVerifyEmailMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "verifyPhone":
		_, err := zclient(ctx, r.Zitadel).Management().SetCustomVerifyPhoneMessageText(ctx, &management.SetCustomVerifyPhoneMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "verifySmsOtp":
		_, err := zclient(ctx, r.Zitadel).Management().SetCustomVerifySMSOTPMessageText(ctx, &management.SetCustomVerifySMSOTPMessageTextRequest{
			Language: spec.Language, Text: spec.Text,
		})
		return err
	case "verifyEmailOtp":
		_, err := zclient(ctx, r.Zitadel).Management().SetCustomVerifyEmailOTPMessageText(ctx, &management.SetCustomVerifyEmailOTPMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "domainClaimed":
		_, err := zclient(ctx, r.Zitadel).Management().SetCustomDomainClaimedMessageCustomText(ctx, &management.SetCustomDomainClaimedMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "passwordlessRegistration":
		_, err := zclient(ctx, r.Zitadel).Management().SetCustomPasswordlessRegistrationMessageCustomText(ctx, &management.SetCustomPasswordlessRegistrationMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "passwordChange":
		_, err := zclient(ctx, r.Zitadel).Management().SetCustomPasswordChangeMessageCustomText(ctx, &management.SetCustomPasswordChangeMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader,
			Subject: spec.Subject, Greeting: spec.Greeting, Text: spec.Text,
			ButtonText: spec.ButtonText, FooterText: spec.FooterText,
		})
		return err
	case "inviteUser":
		_, err := zclient(ctx, r.Zitadel).Management().SetCustomInviteUserMessageCustomText(ctx, &management.SetCustomInviteUserMessageTextRequest{
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
		_, err := zclient(ctx, r.Zitadel).Management().ResetCustomInitMessageTextToDefault(ctx, &management.ResetCustomInitMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "passwordReset":
		_, err := zclient(ctx, r.Zitadel).Management().ResetCustomPasswordResetMessageTextToDefault(ctx, &management.ResetCustomPasswordResetMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifyEmail":
		_, err := zclient(ctx, r.Zitadel).Management().ResetCustomVerifyEmailMessageTextToDefault(ctx, &management.ResetCustomVerifyEmailMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifyPhone":
		_, err := zclient(ctx, r.Zitadel).Management().ResetCustomVerifyPhoneMessageTextToDefault(ctx, &management.ResetCustomVerifyPhoneMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifySmsOtp":
		_, err := zclient(ctx, r.Zitadel).Management().ResetCustomVerifySMSOTPMessageTextToDefault(ctx, &management.ResetCustomVerifySMSOTPMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "verifyEmailOtp":
		_, err := zclient(ctx, r.Zitadel).Management().ResetCustomVerifyEmailOTPMessageTextToDefault(ctx, &management.ResetCustomVerifyEmailOTPMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "domainClaimed":
		_, err := zclient(ctx, r.Zitadel).Management().ResetCustomDomainClaimedMessageTextToDefault(ctx, &management.ResetCustomDomainClaimedMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "passwordlessRegistration":
		_, err := zclient(ctx, r.Zitadel).Management().ResetCustomPasswordlessRegistrationMessageTextToDefault(ctx, &management.ResetCustomPasswordlessRegistrationMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "passwordChange":
		_, err := zclient(ctx, r.Zitadel).Management().ResetCustomPasswordChangeMessageTextToDefault(ctx, &management.ResetCustomPasswordChangeMessageTextToDefaultRequest{Language: spec.Language})
		return err
	case "inviteUser":
		_, err := zclient(ctx, r.Zitadel).Management().ResetCustomInviteUserMessageTextToDefault(ctx, &management.ResetCustomInviteUserMessageTextToDefaultRequest{Language: spec.Language})
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
