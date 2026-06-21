package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// LoginPolicyReconciler reconciles a LoginPolicy object.
type LoginPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=loginpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=loginpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=loginpolicies/finalizers,verbs=update

func (r *LoginPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.LoginPolicy
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
		_, err := r.Zitadel.Management().ResetLoginPolicyToDefault(ctx, &management.ResetLoginPolicyToDefaultRequest{})
		if err != nil && status.Code(err) != codes.NotFound {
			logger.Info("could not reset login policy to default", "error", err)
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

	// Check if custom policy already exists.
	currentResp, err := r.Zitadel.Management().GetLoginPolicy(ctx, &management.GetLoginPolicyRequest{})
	if err != nil && status.Code(err) != codes.NotFound {
		return ctrl.Result{}, fmt.Errorf("getting login policy: %w", err)
	}

	// Determine if a custom policy exists (not the default).
	hasCustomPolicy := currentResp != nil && currentResp.GetPolicy() != nil && !currentResp.GetPolicy().GetIsDefault()

	if hasCustomPolicy {
		// Custom policy exists, update it.
		if err := r.updateCustomPolicy(ctx, &cr.Spec); err != nil {
			return ctrl.Result{}, fmt.Errorf("updating custom login policy: %w", err)
		}
	} else {
		// No custom policy yet, create one.
		if err := r.addCustomPolicy(ctx, &cr.Spec); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding custom login policy: %w", err)
		}
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

	logger.Info("loginpolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *LoginPolicyReconciler) addCustomPolicy(ctx context.Context, spec *zitadelv1alpha2.LoginPolicySpec) error {
	req := &management.AddCustomLoginPolicyRequest{
		AllowUsernamePassword:  boolValue(spec.UserLogin, true),
		AllowExternalIdp:       boolValue(spec.AllowExternalIdp, false),
		AllowRegister:          boolValue(spec.AllowRegister, false),
		ForceMfa:               boolValue(spec.ForceMfa, false),
		ForceMfaLocalOnly:      boolValue(spec.ForceMfaLocalOnly, false),
		HidePasswordReset:      boolValue(spec.HidePasswordReset, false),
		PasswordlessType:       mapPasswordlessType(spec.PasswordlessType),
		AllowDomainDiscovery:   boolValue(spec.AllowDomainDiscovery, false),
		IgnoreUnknownUsernames: boolValue(spec.IgnoreUnknownUsernames, false),
		DefaultRedirectUri:     spec.DefaultRedirectUri,
		DisableLoginWithEmail:  boolValue(spec.DisableLoginWithEmail, false),
		DisableLoginWithPhone:  boolValue(spec.DisableLoginWithPhone, false),
	}

	setLifetimeFields(spec, req)

	_, err := r.Zitadel.Management().AddCustomLoginPolicy(ctx, req)
	return err
}

func (r *LoginPolicyReconciler) updateCustomPolicy(ctx context.Context, spec *zitadelv1alpha2.LoginPolicySpec) error {
	req := &management.UpdateCustomLoginPolicyRequest{
		AllowUsernamePassword:  boolValue(spec.UserLogin, true),
		AllowExternalIdp:       boolValue(spec.AllowExternalIdp, false),
		AllowRegister:          boolValue(spec.AllowRegister, false),
		ForceMfa:               boolValue(spec.ForceMfa, false),
		ForceMfaLocalOnly:      boolValue(spec.ForceMfaLocalOnly, false),
		HidePasswordReset:      boolValue(spec.HidePasswordReset, false),
		PasswordlessType:       mapPasswordlessType(spec.PasswordlessType),
		AllowDomainDiscovery:   boolValue(spec.AllowDomainDiscovery, false),
		IgnoreUnknownUsernames: boolValue(spec.IgnoreUnknownUsernames, false),
		DefaultRedirectUri:     spec.DefaultRedirectUri,
		DisableLoginWithEmail:  boolValue(spec.DisableLoginWithEmail, false),
		DisableLoginWithPhone:  boolValue(spec.DisableLoginWithPhone, false),
	}

	if spec.PasswordCheckLifetime != "" {
		if d, err := time.ParseDuration(spec.PasswordCheckLifetime); err == nil {
			req.PasswordCheckLifetime = durationpb.New(d)
		}
	}
	if spec.ExternalLoginCheckLifetime != "" {
		if d, err := time.ParseDuration(spec.ExternalLoginCheckLifetime); err == nil {
			req.ExternalLoginCheckLifetime = durationpb.New(d)
		}
	}
	if spec.MfaInitSkipLifetime != "" {
		if d, err := time.ParseDuration(spec.MfaInitSkipLifetime); err == nil {
			req.MfaInitSkipLifetime = durationpb.New(d)
		}
	}
	if spec.MultiFactorCheckLifetime != "" {
		if d, err := time.ParseDuration(spec.MultiFactorCheckLifetime); err == nil {
			req.MultiFactorCheckLifetime = durationpb.New(d)
		}
	}
	if spec.SecondFactorCheckLifetime != "" {
		if d, err := time.ParseDuration(spec.SecondFactorCheckLifetime); err == nil {
			req.SecondFactorCheckLifetime = durationpb.New(d)
		}
	}

	_, err := r.Zitadel.Management().UpdateCustomLoginPolicy(ctx, req)
	return err
}

// setLifetimeFields sets lifetime fields on the AddCustomLoginPolicyRequest.
// This is a separate function to avoid code duplication with the proto message type.
func setLifetimeFields(spec *zitadelv1alpha2.LoginPolicySpec, req *management.AddCustomLoginPolicyRequest) {
	if spec.PasswordCheckLifetime != "" {
		if d, err := time.ParseDuration(spec.PasswordCheckLifetime); err == nil {
			req.PasswordCheckLifetime = durationpb.New(d)
		}
	}
	if spec.ExternalLoginCheckLifetime != "" {
		if d, err := time.ParseDuration(spec.ExternalLoginCheckLifetime); err == nil {
			req.ExternalLoginCheckLifetime = durationpb.New(d)
		}
	}
	if spec.MfaInitSkipLifetime != "" {
		if d, err := time.ParseDuration(spec.MfaInitSkipLifetime); err == nil {
			req.MfaInitSkipLifetime = durationpb.New(d)
		}
	}
	if spec.MultiFactorCheckLifetime != "" {
		if d, err := time.ParseDuration(spec.MultiFactorCheckLifetime); err == nil {
			req.MultiFactorCheckLifetime = durationpb.New(d)
		}
	}
	if spec.SecondFactorCheckLifetime != "" {
		if d, err := time.ParseDuration(spec.SecondFactorCheckLifetime); err == nil {
			req.SecondFactorCheckLifetime = durationpb.New(d)
		}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoginPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.LoginPolicy{}).
		Named("loginpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
