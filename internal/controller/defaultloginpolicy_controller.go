package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	policyv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/policy"
)

// DefaultLoginPolicyReconciler reconciles a DefaultLoginPolicy object.
type DefaultLoginPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultloginpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultloginpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultloginpolicies/finalizers,verbs=update

func (r *DefaultLoginPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.DefaultLoginPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion: reset to safe defaults (do not leave instance unprotected).
	if !cr.DeletionTimestamp.IsZero() {
		// Reset to safe defaults — keep userLogin enabled.
		_, _ = r.Zitadel.Admin().UpdateLoginPolicy(ctx, &admin.UpdateLoginPolicyRequest{
			AllowUsernamePassword:  true,
			AllowExternalIdp:       false,
			AllowRegister:          false,
			ForceMfa:               false,
			ForceMfaLocalOnly:      false,
			HidePasswordReset:      false,
			PasswordlessType:       policyv1.PasswordlessType_PASSWORDLESS_TYPE_NOT_ALLOWED,
			AllowDomainDiscovery:   false,
			IgnoreUnknownUsernames: false,
			DefaultRedirectUri:     "",
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

	// Resolve IdP IDs for the idps list.
	idpIDs, err := r.resolveIdpRefs(ctx, &cr)
	if err != nil {
		logger.Info("waiting for idp ref to become ready", "error", err)
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "IdpNotReady", err.Error())
		cr.Status.Ready = false
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{RequeueAfter: requeueOnError}, nil
	}

	// Read current login policy from Zitadel.
	current, err := r.Zitadel.Admin().GetLoginPolicy(ctx, &admin.GetLoginPolicyRequest{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting default login policy: %w", err)
	}

	// Detect drift and update if needed.
	if r.hasDrift(&cr.Spec, current.GetPolicy()) {
		if err := r.updatePolicy(ctx, &cr.Spec); err != nil {
			return ctrl.Result{}, fmt.Errorf("updating default login policy: %w", err)
		}
		logger.Info("default login policy updated (drift detected)")
	}

	// Sync IDPs in the login policy.
	if err := r.syncIdps(ctx, idpIDs); err != nil {
		return ctrl.Result{}, fmt.Errorf("syncing login policy idps: %w", err)
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

	logger.Info("defaultloginpolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *DefaultLoginPolicyReconciler) hasDrift(spec *zitadelv1alpha2.DefaultLoginPolicySpec, current *policyv1.LoginPolicy) bool {
	if current == nil {
		return true
	}
	if spec.UserLogin != nil && *spec.UserLogin != current.GetAllowUsernamePassword() {
		return true
	}
	if spec.AllowExternalIdp != nil && *spec.AllowExternalIdp != current.GetAllowExternalIdp() {
		return true
	}
	if spec.AllowRegister != nil && *spec.AllowRegister != current.GetAllowRegister() {
		return true
	}
	if spec.ForceMfa != nil && *spec.ForceMfa != current.GetForceMfa() {
		return true
	}
	if spec.ForceMfaLocalOnly != nil && *spec.ForceMfaLocalOnly != current.GetForceMfaLocalOnly() {
		return true
	}
	if spec.HidePasswordReset != nil && *spec.HidePasswordReset != current.GetHidePasswordReset() {
		return true
	}
	if spec.PasswordlessType != "" {
		desired := mapPasswordlessType(spec.PasswordlessType)
		if desired != current.GetPasswordlessType() {
			return true
		}
	}
	if spec.AllowDomainDiscovery != nil && *spec.AllowDomainDiscovery != current.GetAllowDomainDiscovery() {
		return true
	}
	if spec.IgnoreUnknownUsernames != nil && *spec.IgnoreUnknownUsernames != current.GetIgnoreUnknownUsernames() {
		return true
	}
	if spec.DefaultRedirectUri != "" && spec.DefaultRedirectUri != current.GetDefaultRedirectUri() {
		return true
	}
	return false
}

func (r *DefaultLoginPolicyReconciler) updatePolicy(ctx context.Context, spec *zitadelv1alpha2.DefaultLoginPolicySpec) error {
	req := &admin.UpdateLoginPolicyRequest{
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

	_, err := r.Zitadel.Admin().UpdateLoginPolicy(ctx, req)
	return err
}

func (r *DefaultLoginPolicyReconciler) resolveIdpRefs(ctx context.Context, cr *zitadelv1alpha2.DefaultLoginPolicy) ([]string, error) {
	var ids []string
	for _, ref := range cr.Spec.Idps {
		if ref.IdpId != "" && ref.IdpRef != nil {
			return nil, fmt.Errorf("idpId and idpRef are mutually exclusive")
		}
		if ref.IdpId != "" {
			ids = append(ids, ref.IdpId)
			continue
		}
		if ref.IdpRef != nil {
			ns := ref.IdpRef.Namespace
			if ns == "" {
				ns = cr.Namespace
			}
			var gidp zitadelv1alpha2.GoogleIdP
			if err := r.Get(ctx, client.ObjectKey{Name: ref.IdpRef.Name, Namespace: ns}, &gidp); err != nil {
				return nil, fmt.Errorf("resolving idpRef %s/%s: %w", ns, ref.IdpRef.Name, err)
			}
			if gidp.Status.IdpID == "" {
				return nil, fmt.Errorf("idpRef %s/%s not yet ready (no idpID in status)", ns, ref.IdpRef.Name)
			}
			ids = append(ids, gidp.Status.IdpID)
			continue
		}
		return nil, fmt.Errorf("each idp entry must specify idpId or idpRef")
	}
	return ids, nil
}

func (r *DefaultLoginPolicyReconciler) syncIdps(ctx context.Context, desiredIDs []string) error {
	// List current IDPs in login policy.
	listResp, err := r.Zitadel.Admin().ListLoginPolicyIDPs(ctx, &admin.ListLoginPolicyIDPsRequest{})
	if err != nil {
		return fmt.Errorf("listing login policy idps: %w", err)
	}

	currentIDs := make(map[string]bool)
	for _, idp := range listResp.GetResult() {
		currentIDs[idp.GetIdpId()] = true
	}

	desiredSet := make(map[string]bool)
	for _, id := range desiredIDs {
		desiredSet[id] = true
	}

	// Add missing.
	for _, id := range desiredIDs {
		if !currentIDs[id] {
			_, err := r.Zitadel.Admin().AddIDPToLoginPolicy(ctx, &admin.AddIDPToLoginPolicyRequest{
				IdpId: id,
			})
			if err != nil {
				return fmt.Errorf("adding idp %s to login policy: %w", id, err)
			}
		}
	}

	// Remove extras.
	for id := range currentIDs {
		if !desiredSet[id] {
			_, err := r.Zitadel.Admin().RemoveIDPFromLoginPolicy(ctx, &admin.RemoveIDPFromLoginPolicyRequest{
				IdpId: id,
			})
			if err != nil {
				return fmt.Errorf("removing idp %s from login policy: %w", id, err)
			}
		}
	}

	return nil
}

func mapPasswordlessType(s string) policyv1.PasswordlessType {
	switch s {
	case "allowed":
		return policyv1.PasswordlessType_PASSWORDLESS_TYPE_ALLOWED
	default:
		return policyv1.PasswordlessType_PASSWORDLESS_TYPE_NOT_ALLOWED
	}
}

func boolValue(ptr *bool, defaultVal bool) bool {
	if ptr != nil {
		return *ptr
	}
	return defaultVal
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultLoginPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultLoginPolicy{}).
		Named("defaultloginpolicy").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
