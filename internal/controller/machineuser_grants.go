package controller

import (
	"context"
	"fmt"
	"slices"

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
// namespace's resolved scope project, via the delegated client — the grant
// can never widen beyond the scope because the delegate's PROJECT_OWNER /
// ORG_OWNER membership is the outer bound the API enforces.
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

	// spec.roles requires a project to grant on: the resolved scope's
	// project (project-scope rule), or a previously recorded one.
	projectID := ""
	switch {
	case rs.delegate != nil && rs.delegate.ProjectID != "":
		projectID = rs.delegate.ProjectID
	case cr.Status.ProjectId != "":
		projectID = cr.Status.ProjectId
	default:
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse,
			"RolesRequireProjectScope",
			"spec.roles needs a project-scoped namespace (a scope-map rule naming a project) to grant on")
		cr.Status.Ready = false
		if err := applyStatus(ctx, r.Client, r.Config, cr); err != nil {
			return true, ctrl.Result{}, err
		}
		return true, ctrl.Result{RequeueAfter: requeueOnError}, nil
	}

	grantID, err := r.syncGrant(ctx, cr, userID, projectID)
	if err != nil {
		return true, ctrl.Result{}, err
	}
	cr.Status.GrantId = grantID
	cr.Status.ProjectId = projectID
	return false, ctrl.Result{}, nil
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
