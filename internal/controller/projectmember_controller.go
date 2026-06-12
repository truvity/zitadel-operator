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
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
)

// ProjectMemberReconciler reconciles a ProjectMember object.
type ProjectMemberReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectmembers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectmembers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectmembers/finalizers,verbs=update

func (r *ProjectMemberReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha1.ProjectMember
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve userId: prefer UserEmail when set, fall back to UserId.
	userID := cr.Spec.UserId
	if cr.Spec.UserEmail != "" {
		resolved, err := resolveUserIDByEmail(ctx, r.Zitadel, cr.Spec.UserEmail)
		if err != nil {
			if isUserNotFoundError(err) {
				logger.Info("user not found in Zitadel, setting UserPending condition", "email", cr.Spec.UserEmail)
				return r.setUserPendingStatus(ctx, &cr)
			}
			return ctrl.Result{}, fmt.Errorf("resolving user email %q: %w", cr.Spec.UserEmail, err)
		}
		userID = resolved
	}

	// Resolve projectRef → projectId.
	projectID, err := r.resolveProjectID(ctx, cr.Spec.ProjectRef)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolving project %q: %w", cr.Spec.ProjectRef, err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		_, err := r.Zitadel.Management().RemoveProjectMember(ctx, &management.RemoveProjectMemberRequest{ //nolint:staticcheck // v1 API required
			ProjectId: projectID,
			UserId:    userID,
		})
		if err != nil && status.Code(err) != codes.NotFound {
			return ctrl.Result{}, fmt.Errorf("removing project member: %w", err)
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

	// Add project member (idempotent — AddProjectMember updates roles if member already exists).
	_, err = r.Zitadel.Management().AddProjectMember(ctx, &management.AddProjectMemberRequest{ //nolint:staticcheck // v1 API required
		ProjectId: projectID,
		UserId:    userID,
		Roles:     cr.Spec.Roles,
	})
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return ctrl.Result{}, fmt.Errorf("adding project member: %w", err)
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	cr.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Synced",
			Message:            fmt.Sprintf("ProjectMember user %q synced to project %q", userID, cr.Spec.ProjectRef),
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("projectmember reconciled", "projectRef", cr.Spec.ProjectRef, "userId", userID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ProjectMemberReconciler) resolveProjectID(ctx context.Context, projectName string) (string, error) {
	listResp, err := r.Zitadel.Project().ListProjects(ctx, &projectv2.ListProjectsRequest{
		Filters: []*projectv2.ProjectSearchFilter{
			{
				Filter: &projectv2.ProjectSearchFilter_ProjectNameFilter{
					ProjectNameFilter: &projectv2.ProjectNameFilter{
						ProjectName: projectName,
						Method:      filterv2.TextFilterMethod_TEXT_FILTER_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}

	for _, p := range listResp.GetProjects() {
		if p.GetName() == projectName {
			return p.GetProjectId(), nil
		}
	}

	return "", fmt.Errorf("project %q not found", projectName)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectMemberReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.ProjectMember{}).
		Named("projectmember").
		WithEventFilter(generationChangedPredicate()).
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}

// setUserPendingStatus sets a UserPending condition and requeues after requeueInterval (5 minutes).
func (r *ProjectMemberReconciler) setUserPendingStatus(ctx context.Context, cr *zitadelv1alpha1.ProjectMember) (ctrl.Result, error) {
	now := metav1.NewTime(time.Now())
	cr.Status.Ready = false
	cr.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "UserPending",
			Message:            fmt.Sprintf("User %q has not logged in yet", cr.Spec.UserEmail),
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, cr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}
