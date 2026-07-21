package controller

import (
	"context"
	"fmt"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/member"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"
)

// ProjectGrantMemberReconciler reconciles a ProjectGrantMember object.
type ProjectGrantMemberReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectgrantmembers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectgrantmembers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectgrantmembers/finalizers,verbs=update

func (r *ProjectGrantMemberReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.ProjectGrantMember
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

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Resolve project ID.
	projectID, _, err := resolveScopedProjectId(ctx, r.Client, rs, cr.Spec.ProjectRef, cr.Spec.ProjectId, cr.Namespace, "", "")
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for project ref to become ready", "error", err)
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "ProjectNotReady", err.Error())
			_ = applyStatus(ctx, r.Client, r.Config, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving project: %w", err)
	}

	// Resolve user ID.
	userID, err := resolveUserId(ctx, r.Client, cr.Spec.UserRef, cr.Spec.UserId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for user ref to become ready", "error", err)
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "UserNotReady", err.Error())
			_ = applyStatus(ctx, r.Client, r.Config, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving user: %w", err)
	}

	grantID := cr.Spec.GrantId

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		_, err := zclient(ctx, r.Zitadel).Management().RemoveProjectGrantMember(ctx, &management.RemoveProjectGrantMemberRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
			ProjectId: projectID,
			GrantId:   grantID,
			UserId:    userID,
		})
		if err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("removing project grant member: %w", err)
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

	// Ensure project grant member exists with correct roles.
	if err := r.ensureProjectGrantMember(ctx, projectID, grantID, userID, cr.Spec.Roles); err != nil {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "SyncFailed", err.Error())
		_ = applyStatus(ctx, r.Client, r.Config, &cr)
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := false // no ID fields beyond ready
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("projectgrantmember reconciled", "projectId", projectID, "grantId", grantID, "userId", userID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ProjectGrantMemberReconciler) ensureProjectGrantMember(ctx context.Context, projectID, grantID, userID string, desiredRoles []string) error {
	// Check if member already exists.
	listResp, err := zclient(ctx, r.Zitadel).Management().ListProjectGrantMembers(ctx, &management.ListProjectGrantMembersRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		ProjectId: projectID,
		GrantId:   grantID,
		Query:     &object.ListQuery{Limit: 100},
		Queries: []*member.SearchQuery{
			{
				Query: &member.SearchQuery_UserIdQuery{
					UserIdQuery: &member.UserIDQuery{
						UserId: userID,
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("listing project grant members: %w", err)
	}

	for _, m := range listResp.GetResult() {
		if m.GetUserId() == userID {
			// Member exists, update roles if needed.
			if !rolesEqual(m.GetRoles(), desiredRoles) {
				_, err := zclient(ctx, r.Zitadel).Management().UpdateProjectGrantMember(ctx, &management.UpdateProjectGrantMemberRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
					ProjectId: projectID,
					GrantId:   grantID,
					UserId:    userID,
					Roles:     desiredRoles,
				})
				if err != nil {
					return fmt.Errorf("updating project grant member: %w", err)
				}
			}
			return nil
		}
	}

	// Create new member.
	_, err = zclient(ctx, r.Zitadel).Management().AddProjectGrantMember(ctx, &management.AddProjectGrantMemberRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		ProjectId: projectID,
		GrantId:   grantID,
		UserId:    userID,
		Roles:     desiredRoles,
	})
	if err != nil {
		return fmt.Errorf("adding project grant member: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectGrantMemberReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.ProjectGrantMember{}).
		Named("projectgrantmember").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
