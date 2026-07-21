package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// LoginPolicyReconciler reconciles a LoginPolicy object.
type LoginPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
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

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		_, err := zclient(ctx, r.Zitadel).Management().ResetLoginPolicyToDefault(ctx, &management.ResetLoginPolicyToDefaultRequest{})
		if err != nil && status.Code(err) != codes.NotFound {
			return err
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

	// Business logic.
	if err := r.reconcileSpec(ctx, &cr.Spec); err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := cr.Status.OrganizationId != orgID
	cr.Status.OrganizationId = orgID
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("loginpolicy reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *LoginPolicyReconciler) reconcileSpec(ctx context.Context, spec *zitadelv1alpha2.LoginPolicySpec) error {
	currentResp, err := zclient(ctx, r.Zitadel).Management().GetLoginPolicy(ctx, &management.GetLoginPolicyRequest{})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("getting login policy: %w", err)
	}

	hasCustomPolicy := currentResp != nil && currentResp.GetPolicy() != nil && !currentResp.GetPolicy().GetIsDefault()

	if hasCustomPolicy {
		return r.updateCustomPolicy(ctx, spec)
	}
	return r.addCustomPolicy(ctx, spec)
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

	_, err := zclient(ctx, r.Zitadel).Management().AddCustomLoginPolicy(ctx, req)
	if err != nil {
		return fmt.Errorf("adding custom login policy: %w", err)
	}
	return nil
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

	_, err := zclient(ctx, r.Zitadel).Management().UpdateCustomLoginPolicy(ctx, req)
	if err != nil {
		return fmt.Errorf("updating custom login policy: %w", err)
	}
	return nil
}

// setLifetimeFields sets lifetime fields on the AddCustomLoginPolicyRequest.
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
