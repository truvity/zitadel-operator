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
	"github.com/truvity/zitadel-operator/internal/zitadel"
)

// ProjectMemberReconciler reconciles a ProjectMember object.
type ProjectMemberReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectmembers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectmembers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectmembers/finalizers,verbs=update

func (r *ProjectMemberReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.ProjectMember
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

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Resolve project ID.
	projectID, _, err := resolveProjectId(ctx, r.Client, cr.Spec.ProjectRef, cr.Spec.ProjectId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for project ref to become ready", "error", err)
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "ProjectNotReady", err.Error())
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving project: %w", err)
	}

	// Resolve user ID.
	userID, err := r.resolveUserID(ctx, &cr)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for user ref to become ready", "error", err)
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "UserNotReady", err.Error())
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving user: %w", err)
	}

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		_, err := r.Zitadel.Management().RemoveProjectMember(ctx, &management.RemoveProjectMemberRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
			ProjectId: projectID,
			UserId:    userID,
		})
		if err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("removing project member: %w", err)
		}
		return nil
	}); done {
		return result, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure project member exists with correct roles.
	if err := r.ensureProjectMember(ctx, projectID, userID, cr.Spec.Roles); err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := false // no ID fields beyond ready
	if err := markReady(ctx, r.Client, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("projectmember reconciled", "projectId", projectID, "userId", userID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ProjectMemberReconciler) resolveUserID(ctx context.Context, cr *zitadelv1alpha2.ProjectMember) (string, error) {
	if cr.Spec.UserId != "" && cr.Spec.UserRef != nil {
		return "", fmt.Errorf("userId and userRef are mutually exclusive")
	}
	if cr.Spec.UserId == "" && cr.Spec.UserRef == nil {
		return "", fmt.Errorf("one of userId or userRef is required")
	}
	if cr.Spec.UserId != "" {
		return cr.Spec.UserId, nil
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

func (r *ProjectMemberReconciler) ensureProjectMember(ctx context.Context, projectID, userID string, desiredRoles []string) error {
	// Check if member already exists.
	listResp, err := r.Zitadel.Management().ListProjectMembers(ctx, &management.ListProjectMembersRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		ProjectId: projectID,
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
		return fmt.Errorf("listing project members: %w", err)
	}

	if listResp != nil {
		for _, m := range listResp.GetResult() {
			if m.GetUserId() == userID {
				// Member exists, update roles if needed.
				if !rolesEqual(m.GetRoles(), desiredRoles) {
					_, err := r.Zitadel.Management().UpdateProjectMember(ctx, &management.UpdateProjectMemberRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
						ProjectId: projectID,
						UserId:    userID,
						Roles:     desiredRoles,
					})
					if err != nil {
						return fmt.Errorf("updating project member: %w", err)
					}
				}
				return nil
			}
		}
	}

	// Create new member.
	_, err = r.Zitadel.Management().AddProjectMember(ctx, &management.AddProjectMemberRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		ProjectId: projectID,
		UserId:    userID,
		Roles:     desiredRoles,
	})
	if err != nil {
		return fmt.Errorf("adding project member: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectMemberReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.ProjectMember{}).
		Named("projectmember").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
