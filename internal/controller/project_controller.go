package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
)

// ProjectReconciler reconciles a Project object.
type ProjectReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projects/finalizers,verbs=update

func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Project CR.
	var cr zitadelv1alpha1.Project
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.ProjectId != "" {
			_, err := r.Zitadel.Project().DeleteProject(ctx, &projectv2.DeleteProjectRequest{
				ProjectId: cr.Status.ProjectId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("deleting project: %w", err)
			}
		}
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

	// Create or get project by name.
	projectID, err := r.ensureProject(ctx, &cr)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync roles.
	if err := r.syncRoles(ctx, projectID, cr.Spec.Roles); err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.ProjectId = projectID
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("project reconciled", "projectId", projectID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ProjectReconciler) ensureProject(ctx context.Context, cr *zitadelv1alpha1.Project) (string, error) {
	// If we already have a project ID, verify it still exists.
	if cr.Status.ProjectId != "" {
		_, err := r.Zitadel.Project().GetProject(ctx, &projectv2.GetProjectRequest{
			ProjectId: cr.Status.ProjectId,
		})
		if err == nil {
			return cr.Status.ProjectId, nil
		}
		if status.Code(err) != codes.NotFound {
			return "", fmt.Errorf("getting project: %w", err)
		}
		// Project was deleted externally, recreate it.
	}

	// Resolve organization ID.
	orgID := cr.Spec.OrganizationId
	if orgID == "" {
		// Use the default organization (first in the list).
		orgs, err := r.Zitadel.Organization().ListOrganizations(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("listing organizations to resolve default: %w", err)
		}
		if len(orgs.GetResult()) == 0 {
			return "", fmt.Errorf("no organizations found")
		}
		orgID = orgs.GetResult()[0].GetId()
	}

	// Search by name.
	listResp, err := r.Zitadel.Project().ListProjects(ctx, &projectv2.ListProjectsRequest{
		Filters: []*projectv2.ProjectSearchFilter{
			{
				Filter: &projectv2.ProjectSearchFilter_ProjectNameFilter{
					ProjectNameFilter: &projectv2.ProjectNameFilter{
						ProjectName: cr.Name,
						Method:      filterv2.TextFilterMethod_TEXT_FILTER_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("listing projects: %w", err)
	}

	for _, p := range listResp.GetProjects() {
		if p.GetName() == cr.Name {
			return p.GetProjectId(), nil
		}
	}

	// Create new project.
	createResp, err := r.Zitadel.Project().CreateProject(ctx, &projectv2.CreateProjectRequest{
		OrganizationId:       orgID,
		Name:                 cr.Name,
		ProjectRoleAssertion: cr.Spec.AssertRolesOnAuth,
	})
	if err != nil {
		return "", fmt.Errorf("creating project: %w", err)
	}

	return createResp.GetProjectId(), nil
}

func (r *ProjectReconciler) syncRoles(ctx context.Context, projectID string, desiredRoles []string) error {
	// List existing roles.
	listResp, err := r.Zitadel.Project().ListProjectRoles(ctx, &projectv2.ListProjectRolesRequest{
		ProjectId: projectID,
	})
	if err != nil {
		return fmt.Errorf("listing project roles: %w", err)
	}

	existingRoles := make(map[string]bool)
	for _, role := range listResp.GetProjectRoles() {
		existingRoles[role.GetKey()] = true
	}

	desiredSet := make(map[string]bool)
	for _, role := range desiredRoles {
		desiredSet[role] = true
	}

	// Add missing roles.
	for _, role := range desiredRoles {
		if !existingRoles[role] {
			_, err := r.Zitadel.Project().AddProjectRole(ctx, &projectv2.AddProjectRoleRequest{
				ProjectId:   projectID,
				RoleKey:     role,
				DisplayName: role,
			})
			if err != nil && status.Code(err) != codes.AlreadyExists {
				return fmt.Errorf("adding role %s: %w", role, err)
			}
		}
	}

	// Remove extra roles.
	for role := range existingRoles {
		if !desiredSet[role] {
			_, err := r.Zitadel.Project().RemoveProjectRole(ctx, &projectv2.RemoveProjectRoleRequest{
				ProjectId: projectID,
				RoleKey:   role,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return fmt.Errorf("removing role %s: %w", role, err)
			}
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.Project{}).
		Named("project").
		Complete(r)
}
