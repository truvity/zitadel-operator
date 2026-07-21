package controller

import (
	"context"
	"fmt"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"
)

// OrgMemberReconciler reconciles an OrgMember object.
type OrgMemberReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=orgmembers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=orgmembers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=orgmembers/finalizers,verbs=update

func (r *OrgMemberReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.OrgMember
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
		if isRefNotReady(err) {
			logger.Info("waiting for organization ref to become ready", "error", err)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Resolve user ID.
	userID, err := resolveUserIdIncludingHuman(ctx, r.Client, cr.Spec.UserRef, cr.Spec.UserId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for user ref to become ready", "error", err)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving user: %w", err)
	}

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		_, err := zclient(ctx, r.Zitadel).Management().RemoveOrgMember(ctx, &management.RemoveOrgMemberRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
			UserId: userID,
		})
		if err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("removing org member: %w", err)
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

	// Ensure org member exists — try update first, then add.
	_, err = zclient(ctx, r.Zitadel).Management().UpdateOrgMember(ctx, &management.UpdateOrgMemberRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		UserId: userID,
		Roles:  cr.Spec.Roles,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Member doesn't exist, add it.
			_, err = zclient(ctx, r.Zitadel).Management().AddOrgMember(ctx, &management.AddOrgMemberRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
				UserId: userID,
				Roles:  cr.Spec.Roles,
			})
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("adding org member: %w", err)
			}
		} else {
			return ctrl.Result{}, fmt.Errorf("updating org member: %w", err)
		}
	}

	// Status.
	statusChanged := cr.Status.OrganizationId != orgID
	cr.Status.OrganizationId = orgID
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("orgmember reconciled", "userId", userID, "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// resolveUserIdIncludingHuman resolves a user ID from either a UserRef or explicit UserId.
// Unlike resolveUserId in common.go which only checks MachineUser,
// this also checks HumanUser.
func resolveUserIdIncludingHuman(ctx context.Context, k8s client.Client, ref *zitadelv1alpha2.ResourceRef, explicitID, sourceNamespace string) (string, error) {
	if ref != nil && explicitID != "" {
		return "", fmt.Errorf("userRef and userId are mutually exclusive")
	}

	if ref == nil && explicitID == "" {
		return "", fmt.Errorf("one of userRef or userId is required")
	}

	if explicitID != "" {
		return explicitID, nil
	}

	ns := ref.Namespace
	if ns == "" {
		ns = sourceNamespace
	}

	// Try MachineUser first.
	var mu zitadelv1alpha2.MachineUser
	if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, &mu); err == nil {
		if mu.Status.UserId == "" {
			return "", fmt.Errorf("userRef %s/%s not yet ready (no userId in status)", ns, ref.Name)
		}
		return mu.Status.UserId, nil
	}

	// Try HumanUser.
	var hu zitadelv1alpha2.HumanUser
	if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, &hu); err == nil {
		if hu.Status.UserId == "" {
			return "", fmt.Errorf("userRef %s/%s not yet ready (no userId in status)", ns, ref.Name)
		}
		return hu.Status.UserId, nil
	}

	return "", fmt.Errorf("userRef %s/%s not found (tried MachineUser and HumanUser)", ns, ref.Name)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrgMemberReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.OrgMember{}).
		Named("orgmember").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
