package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
)

// ProjectReconciler reconciles a Project object.
type ProjectReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projects/finalizers,verbs=update

func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.Project
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve organization.
	orgID, err := resolveOrganizationId(ctx, r.Client, r.Config, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for organization ref to become ready", "error", err)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
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

	// Ensure project exists.
	projectID, err := r.ensureProject(ctx, &cr, orgID)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync project roles if specified.
	if len(cr.Spec.Roles) > 0 {
		if err := r.syncRoles(ctx, projectID, orgID, cr.Spec.Roles); err != nil {
			return ctrl.Result{}, fmt.Errorf("syncing roles: %w", err)
		}
	}

	// Update status only if changed.
	if cr.Status.ProjectId != projectID || cr.Status.OrganizationId != orgID || !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.ProjectId = projectID
		cr.Status.OrganizationId = orgID
		cr.Status.Ready = true
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		cr.Status.LastSyncTime = &now
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("project reconciled", "projectId", projectID, "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ProjectReconciler) ensureProject(ctx context.Context, cr *zitadelv1alpha2.Project, orgID string) (string, error) {
	displayName := cr.DisplayName()

	// If we already have a project ID, verify it still exists.
	if cr.Status.ProjectId != "" {
		listResp, err := r.Zitadel.Project().ListProjects(ctx, &projectv2.ListProjectsRequest{
			Filters: []*projectv2.ProjectSearchFilter{
				{
					Filter: &projectv2.ProjectSearchFilter_ProjectNameFilter{
						ProjectNameFilter: &projectv2.ProjectNameFilter{
							ProjectName: displayName,
							Method:      filterv2.TextFilterMethod_TEXT_FILTER_METHOD_EQUALS,
						},
					},
				},
			},
		})
		if err == nil {
			for _, p := range listResp.GetProjects() {
				if p.GetProjectId() == cr.Status.ProjectId {
					return cr.Status.ProjectId, nil
				}
			}
		}
	}

	// Search by name.
	listResp, err := r.Zitadel.Project().ListProjects(ctx, &projectv2.ListProjectsRequest{
		Filters: []*projectv2.ProjectSearchFilter{
			{
				Filter: &projectv2.ProjectSearchFilter_ProjectNameFilter{
					ProjectNameFilter: &projectv2.ProjectNameFilter{
						ProjectName: displayName,
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
		if p.GetName() == displayName {
			return p.GetProjectId(), nil
		}
	}

	// Create new project.
	createResp, err := r.Zitadel.Project().CreateProject(ctx, &projectv2.CreateProjectRequest{
		Name:           displayName,
		OrganizationId: orgID,
	})
	if err != nil {
		return "", fmt.Errorf("creating project: %w", err)
	}

	return createResp.GetProjectId(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.Project{}).
		Named("project").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}

// syncRoles ensures the project has exactly the specified roles.
// It adds missing roles and removes extra ones.
func (r *ProjectReconciler) syncRoles(ctx context.Context, projectID, orgID string, desiredRoles []string) error {
	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// List existing roles.
	listResp, err := r.Zitadel.Management().ListProjectRoles(ctx, &management.ListProjectRolesRequest{ //nolint:staticcheck // no v2 equivalent
		ProjectId: projectID,
		Query:     &object.ListQuery{Limit: 100},
	})
	if err != nil {
		return fmt.Errorf("listing project roles: %w", err)
	}

	// Build set of existing role keys.
	existing := make(map[string]bool, len(listResp.GetResult()))
	for _, role := range listResp.GetResult() {
		existing[role.GetKey()] = true
	}

	// Build set of desired role keys.
	desired := make(map[string]bool, len(desiredRoles))
	for _, role := range desiredRoles {
		desired[role] = true
	}

	// Add missing roles.
	for _, role := range desiredRoles {
		if !existing[role] {
			_, err := r.Zitadel.Management().AddProjectRole(ctx, &management.AddProjectRoleRequest{ //nolint:staticcheck // no v2 equivalent
				ProjectId:   projectID,
				RoleKey:     role,
				DisplayName: role,
			})
			if err != nil {
				return fmt.Errorf("adding role %q: %w", role, err)
			}
		}
	}

	// Remove extra roles.
	for roleKey := range existing {
		if !desired[roleKey] {
			_, err := r.Zitadel.Management().RemoveProjectRole(ctx, &management.RemoveProjectRoleRequest{ //nolint:staticcheck // no v2 equivalent
				ProjectId: projectID,
				RoleKey:   roleKey,
			})
			if err != nil {
				return fmt.Errorf("removing role %q: %w", roleKey, err)
			}
		}
	}

	return nil
}
