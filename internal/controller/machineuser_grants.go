package controller

import (
	"context"
	"fmt"
	"slices"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	userv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user"
)

// ensureRoleGrant syncs spec.roles (INF-426) as a Zitadel user grant on the
// resolved grant target: an explicit spec.projectRef/projectId (v0.19), or
// the namespace's resolved scope project. Under scope maps the grant goes
// through the delegated client and can never widen beyond the scope because
// the delegate's PROJECT_OWNER / ORG_OWNER membership is the outer bound the
// API enforces.
//
// done=true means the caller must return (result, err) immediately.
func (r *MachineUserReconciler) ensureRoleGrant(ctx context.Context, cr *zitadelv1alpha2.MachineUser, rs resolvedScope, userID string) (done bool, result ctrl.Result, err error) {
	// Roles removed from spec: drop the grant we manage.
	if len(cr.Spec.Roles) == 0 {
		if cr.Status.GrantId != "" && cr.Status.ProjectId != "" {
			_, rmErr := zclient(ctx, r.Zitadel).Management().RemoveUserGrant(ctx, &management.RemoveUserGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method
				GrantId: cr.Status.GrantId,
				UserId:  userID,
			})
			if rmErr != nil && status.Code(rmErr) != codes.NotFound {
				return true, ctrl.Result{}, fmt.Errorf("removing role grant: %w", rmErr)
			}
			cr.Status.GrantId = ""
			cr.Status.ProjectId = ""
		}
		return false, ctrl.Result{}, nil
	}

	// spec.roles requires a project to grant on: an explicitly named project
	// (projectRef/projectId, v0.19 — the fleet shape without scope maps),
	// the resolved scope's project (project-scope rule), or a previously
	// recorded one.
	projectID, done, result, err := r.resolveGrantTarget(ctx, cr, rs)
	if done {
		return true, result, err
	}

	// A changed grant target (e.g. projectRef switched) drops the grant on
	// the previous project before creating the new one.
	if cr.Status.GrantId != "" && cr.Status.ProjectId != "" && cr.Status.ProjectId != projectID {
		_, rmErr := zclient(ctx, r.Zitadel).Management().RemoveUserGrant(ctx, &management.RemoveUserGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method
			GrantId: cr.Status.GrantId,
			UserId:  userID,
		})
		if rmErr != nil && status.Code(rmErr) != codes.NotFound {
			return true, ctrl.Result{}, fmt.Errorf("removing role grant on previous project: %w", rmErr)
		}
		cr.Status.GrantId = ""
	}

	grantID, err := r.syncGrant(ctx, cr, userID, projectID)
	if err != nil {
		return true, ctrl.Result{}, err
	}
	cr.Status.GrantId = grantID
	cr.Status.ProjectId = projectID
	return false, ctrl.Result{}, nil
}

// resolveGrantTarget resolves the project the role grant is created on, in
// order of precedence: explicit spec.projectRef/projectId (v0.19), the scope's
// project, a previously recorded status.projectId. Resolution failures become
// conditions: ProjectNotReady requeues politely, InvalidSpec (mutual
// exclusion) and RolesRequireProjectScope fail closed.
// done=true means the caller must return immediately.
func (r *MachineUserReconciler) resolveGrantTarget(ctx context.Context, cr *zitadelv1alpha2.MachineUser, rs resolvedScope) (projectID string, done bool, result ctrl.Result, err error) {
	switch {
	case cr.Spec.ProjectRef != nil || cr.Spec.ProjectId != "":
		pid, _, resErr := resolveProjectId(ctx, r.Client, cr.Spec.ProjectRef, cr.Spec.ProjectId, cr.Namespace)
		if resErr == nil {
			return pid, false, ctrl.Result{}, nil
		}
		if isRefNotReady(resErr) || isRefNotFound(resErr) {
			log.FromContext(ctx).Info("waiting for project ref to become ready", "error", resErr)
			result, err := r.failGrantTarget(ctx, cr, "ProjectNotReady", resErr.Error(), requeueOnError)
			return "", true, result, err
		}
		// Mutual exclusion is a steady-state spec error: fail closed.
		result, err := r.failGrantTarget(ctx, cr, "InvalidSpec", resErr.Error(), requeueInterval)
		return "", true, result, err
	case rs.delegate != nil && rs.delegate.ProjectID != "":
		return rs.delegate.ProjectID, false, ctrl.Result{}, nil
	case cr.Status.ProjectId != "":
		return cr.Status.ProjectId, false, ctrl.Result{}, nil
	default:
		result, err := r.failGrantTarget(ctx, cr, "RolesRequireProjectScope",
			"spec.roles needs a grant target: set spec.projectRef/projectId, or use a project-scoped namespace (a scope-map rule naming a project)",
			requeueOnError)
		return "", true, result, err
	}
}

// failGrantTarget records a Ready=False condition for an unresolvable grant
// target; the caller always returns done=true with the returned values.
func (r *MachineUserReconciler) failGrantTarget(ctx context.Context, cr *zitadelv1alpha2.MachineUser, reason, message string, requeue time.Duration) (ctrl.Result, error) {
	setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, reason, message)
	cr.Status.Ready = false
	if applyErr := applyStatus(ctx, r.Client, r.Config, cr); applyErr != nil {
		return ctrl.Result{}, applyErr
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

// syncGrant finds or creates the user grant and reconciles its role set.
func (r *MachineUserReconciler) syncGrant(ctx context.Context, cr *zitadelv1alpha2.MachineUser, userID, projectID string) (string, error) {
	listResp, err := zclient(ctx, r.Zitadel).Management().ListUserGrants(ctx, &management.ListUserGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method
		Queries: []*userv1.UserGrantQuery{
			{Query: &userv1.UserGrantQuery_UserIdQuery{UserIdQuery: &userv1.UserGrantUserIDQuery{UserId: userID}}},
			{Query: &userv1.UserGrantQuery_ProjectIdQuery{ProjectIdQuery: &userv1.UserGrantProjectIDQuery{ProjectId: projectID}}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("listing user grants: %w", err)
	}

	desired := append([]string(nil), cr.Spec.Roles...)
	slices.Sort(desired)

	if grants := listResp.GetResult(); len(grants) > 0 {
		g := grants[0]
		current := append([]string(nil), g.GetRoleKeys()...)
		slices.Sort(current)
		if !slices.Equal(current, desired) {
			_, err := zclient(ctx, r.Zitadel).Management().UpdateUserGrant(ctx, &management.UpdateUserGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method
				GrantId:  g.GetId(),
				UserId:   userID,
				RoleKeys: cr.Spec.Roles,
			})
			if err != nil {
				return "", fmt.Errorf("updating role grant: %w", err)
			}
			log.FromContext(ctx).Info("machineuser role grant updated", "grantId", g.GetId(), "roles", cr.Spec.Roles)
		}
		return g.GetId(), nil
	}

	addResp, err := zclient(ctx, r.Zitadel).Management().AddUserGrant(ctx, &management.AddUserGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method
		UserId:    userID,
		ProjectId: projectID,
		RoleKeys:  cr.Spec.Roles,
	})
	if err != nil {
		return "", fmt.Errorf("creating role grant: %w", err)
	}
	log.FromContext(ctx).Info("machineuser role grant created", "grantId", addResp.GetUserGrantId(), "roles", cr.Spec.Roles)
	return addResp.GetUserGrantId(), nil
}
