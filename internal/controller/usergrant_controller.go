package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"
)

// UserGrantReconciler reconciles a UserGrant object.
type UserGrantReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=usergrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=usergrants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=usergrants/finalizers,verbs=update

func (r *UserGrantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cr zitadelv1alpha2.UserGrant
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve organization.
	orgID, err := resolveOrganizationId(ctx, r.Client, r.Config, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, &cr, &cr.Status.Conditions, "OrgNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Resolve user ID.
	userID, err := r.resolveUserID(ctx, &cr)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, &cr, &cr.Status.Conditions, "UserNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving user: %w", err)
	}

	// Resolve project ID.
	projectID, _, err := resolveProjectId(ctx, r.Client, cr.Spec.ProjectRef, cr.Spec.ProjectId, cr.Namespace)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, &cr, &cr.Status.Conditions, "ProjectNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving project: %w", err)
	}

	// Deletion.
	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		return r.deleteGrant(ctx, userID, cr.Status.GrantId)
	}); done {
		return result, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure user grant exists with correct roles.
	grantID, err := r.ensureUserGrant(ctx, userID, projectID, cr.Spec.RoleKeys, cr.Status.GrantId)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := cr.Status.GrantId != grantID
	cr.Status.GrantId = grantID
	if err := markReady(ctx, r.Client, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *UserGrantReconciler) deleteGrant(ctx context.Context, userID, grantID string) error {
	if grantID == "" {
		return nil
	}
	_, err := r.Zitadel.Management().RemoveUserGrant(ctx, &management.RemoveUserGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		UserId:  userID,
		GrantId: grantID,
	})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("removing user grant: %w", err)
	}
	return nil
}

func (r *UserGrantReconciler) resolveUserID(ctx context.Context, cr *zitadelv1alpha2.UserGrant) (string, error) {
	if cr.Spec.UserID != "" && cr.Spec.UserRef != nil {
		return "", fmt.Errorf("userId and userRef are mutually exclusive")
	}
	if cr.Spec.UserID == "" && cr.Spec.UserRef == nil {
		return "", fmt.Errorf("one of userId or userRef is required")
	}
	if cr.Spec.UserID != "" {
		return cr.Spec.UserID, nil
	}

	ns := cr.Spec.UserRef.Namespace
	if ns == "" {
		ns = cr.Namespace
	}
	var mu zitadelv1alpha2.MachineUser
	if err := r.Get(ctx, client.ObjectKey{Name: cr.Spec.UserRef.Name, Namespace: ns}, &mu); err != nil {
		return "", fmt.Errorf("resolving userRef %s/%s: %w", ns, cr.Spec.UserRef.Name, err)
	}
	if mu.Status.UserId == "" {
		return "", fmt.Errorf("userRef %s/%s not yet ready (no userId in status)", ns, cr.Spec.UserRef.Name)
	}
	return mu.Status.UserId, nil
}

func (r *UserGrantReconciler) ensureUserGrant(ctx context.Context, userID, projectID string, desiredRoles []string, existingGrantID string) (string, error) {
	// If we have an existing grant ID, check if it still exists and update roles if needed.
	if existingGrantID != "" {
		resp, err := r.Zitadel.Management().GetUserGrantByID(ctx, &management.GetUserGrantByIDRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
			UserId:  userID,
			GrantId: existingGrantID,
		})
		if err == nil {
			// Grant exists, check if roles need updating.
			if !rolesEqual(resp.GetUserGrant().GetRoleKeys(), desiredRoles) {
				_, err := r.Zitadel.Management().UpdateUserGrant(ctx, &management.UpdateUserGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
					UserId:   userID,
					GrantId:  existingGrantID,
					RoleKeys: desiredRoles,
				})
				if err != nil {
					return "", fmt.Errorf("updating user grant: %w", err)
				}
			}
			return existingGrantID, nil
		}
		if status.Code(err) != codes.NotFound {
			return "", fmt.Errorf("getting user grant: %w", err)
		}
		// Grant was deleted externally, search or recreate.
	}

	// Search for existing grant by user+project.
	listResp, err := r.Zitadel.Management().ListUserGrants(ctx, &management.ListUserGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		Query: &object.ListQuery{Limit: 100},
		Queries: []*user.UserGrantQuery{
			{
				Query: &user.UserGrantQuery_UserIdQuery{
					UserIdQuery: &user.UserGrantUserIDQuery{
						UserId: userID,
					},
				},
			},
			{
				Query: &user.UserGrantQuery_ProjectIdQuery{
					ProjectIdQuery: &user.UserGrantProjectIDQuery{
						ProjectId: projectID,
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("listing user grants: %w", err)
	}

	for _, grant := range listResp.GetResult() {
		if grant.GetProjectId() == projectID && grant.GetUserId() == userID {
			if !rolesEqual(grant.GetRoleKeys(), desiredRoles) {
				_, err := r.Zitadel.Management().UpdateUserGrant(ctx, &management.UpdateUserGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
					UserId:   userID,
					GrantId:  grant.GetId(),
					RoleKeys: desiredRoles,
				})
				if err != nil {
					return "", fmt.Errorf("updating user grant: %w", err)
				}
			}
			return grant.GetId(), nil
		}
	}

	// Create new grant.
	addResp, err := r.Zitadel.Management().AddUserGrant(ctx, &management.AddUserGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		UserId:    userID,
		ProjectId: projectID,
		RoleKeys:  desiredRoles,
	})
	if err != nil {
		return "", fmt.Errorf("adding user grant: %w", err)
	}

	return addResp.GetUserGrantId(), nil
}

// rolesEqual compares two role slices (order-independent).
func rolesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aSorted := make([]string, len(a))
	bSorted := make([]string, len(b))
	copy(aSorted, a)
	copy(bSorted, b)
	sort.Strings(aSorted)
	sort.Strings(bSorted)
	return strings.Join(aSorted, ",") == strings.Join(bSorted, ",")
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserGrantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.UserGrant{}).
		Named("usergrant").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
