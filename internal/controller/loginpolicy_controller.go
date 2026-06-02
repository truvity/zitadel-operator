package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
)

// LoginPolicyReconciler reconciles a LoginPolicy object.
type LoginPolicyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=loginpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=loginpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=loginpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=identityproviders,verbs=get;list;watch

func (r *LoginPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the LoginPolicy CR.
	var cr zitadelv1alpha1.LoginPolicy
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion — no cleanup needed, just remove finalizer.
	if !cr.DeletionTimestamp.IsZero() {
		if removeFinalizer(&cr) {
			if err := r.Update(ctx, &cr); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present.
	if addFinalizer(&cr) {
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update login policy settings via Admin service.
	// Ignore FailedPrecondition errors which mean "not changed".
	_, err := r.Zitadel.Admin().UpdateLoginPolicy(ctx, &admin.UpdateLoginPolicyRequest{
		AllowUsernamePassword: true,
		AllowRegister:         cr.Spec.AllowRegister,
		AllowExternalIdp:      cr.Spec.AllowExternalIdp,
		HidePasswordReset:     cr.Spec.HidePasswordReset,
		DisableLoginWithEmail: cr.Spec.DisableLoginWithEmail,
	})
	if err != nil && status.Code(err) != codes.FailedPrecondition {
		return ctrl.Result{}, fmt.Errorf("updating login policy: %w", err)
	}

	// Sync IdP providers on the login policy.
	if err := r.syncIdpProviders(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("loginpolicy reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// syncIdpProviders ensures the login policy has exactly the IdPs specified in the CR.
func (r *LoginPolicyReconciler) syncIdpProviders(ctx context.Context, cr *zitadelv1alpha1.LoginPolicy) error {
	// Resolve desired IdP IDs from IdentityProvider CR names.
	desiredIDs := make(map[string]struct{})
	for _, name := range cr.Spec.IdpProviders {
		var idpCR zitadelv1alpha1.IdentityProvider
		if err := r.Get(ctx, types.NamespacedName{Name: name}, &idpCR); err != nil {
			return fmt.Errorf("looking up IdentityProvider CR %q: %w", name, err)
		}

		idpID := idpCR.Status.IdpId
		if idpID == "" {
			return fmt.Errorf("IdentityProvider CR %q has no idpId in status (not reconciled yet)", name)
		}

		desiredIDs[idpID] = struct{}{}
	}

	// List current IdPs on the login policy.
	listResp, err := r.Zitadel.Admin().ListLoginPolicyIDPs(ctx, &admin.ListLoginPolicyIDPsRequest{})
	if err != nil {
		return fmt.Errorf("listing login policy IDPs: %w", err)
	}

	currentIDs := make(map[string]struct{})
	for _, item := range listResp.GetResult() {
		currentIDs[item.GetIdpId()] = struct{}{}
	}

	// Add desired IdPs not currently present.
	for id := range desiredIDs {
		if _, exists := currentIDs[id]; exists {
			continue
		}
		_, err := r.Zitadel.Admin().AddIDPToLoginPolicy(ctx, &admin.AddIDPToLoginPolicyRequest{
			IdpId: id,
		})
		if err != nil && status.Code(err) != codes.AlreadyExists {
			return fmt.Errorf("adding IDP %q to login policy: %w", id, err)
		}
	}

	// Remove current IdPs not in desired set.
	for id := range currentIDs {
		if _, desired := desiredIDs[id]; desired {
			continue
		}
		_, err := r.Zitadel.Admin().RemoveIDPFromLoginPolicy(ctx, &admin.RemoveIDPFromLoginPolicyRequest{
			IdpId: id,
		})
		if err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("removing IDP %q from login policy: %w", id, err)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoginPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.LoginPolicy{}).
		Named("loginpolicy").
		Complete(r)
}
