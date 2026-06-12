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

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
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

	// Update status only if changed.
	if cr.Status.ProjectId != projectID || cr.Status.OrganizationId != orgID || !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.ProjectId = projectID
		cr.Status.OrganizationId = orgID
		cr.Status.Ready = true
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
