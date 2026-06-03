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

// UserGrantReconciler reconciles a UserGrant object.
type UserGrantReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=usergrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=usergrants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=usergrants/finalizers,verbs=update

func (r *UserGrantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha1.UserGrant
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.GrantId != "" {
			// Resolve user email for deletion.
			userID, resolveErr := resolveUserIDByEmail(ctx, r.Zitadel, cr.Spec.UserEmail)
			if resolveErr == nil {
				_, err := r.Zitadel.Management().RemoveUserGrant(ctx, &management.RemoveUserGrantRequest{ //nolint:staticcheck // v1 API required
					UserId:  userID,
					GrantId: cr.Status.GrantId,
				})
				if err != nil && status.Code(err) != codes.NotFound {
					return ctrl.Result{}, fmt.Errorf("removing user grant: %w", err)
				}
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

	// Resolve userEmail → userId.
	userID, err := resolveUserIDByEmail(ctx, r.Zitadel, cr.Spec.UserEmail)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolving user email %q: %w", cr.Spec.UserEmail, err)
	}

	// Resolve projectRef → projectId.
	projectID, err := r.resolveProjectID(ctx, cr.Spec.ProjectRef)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolving project %q: %w", cr.Spec.ProjectRef, err)
	}

	// Create or update grant.
	grantID := cr.Status.GrantId
	if grantID == "" {
		// Create new user grant.
		resp, err := r.Zitadel.Management().AddUserGrant(ctx, &management.AddUserGrantRequest{ //nolint:staticcheck // v1 API required
			UserId:    userID,
			ProjectId: projectID,
			RoleKeys:  cr.Spec.RoleKeys,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("adding user grant: %w", err)
		}
		grantID = resp.GetUserGrantId()
	} else {
		// Update existing grant.
		_, err := r.Zitadel.Management().UpdateUserGrant(ctx, &management.UpdateUserGrantRequest{ //nolint:staticcheck // v1 API required
			GrantId:  grantID,
			UserId:   userID,
			RoleKeys: cr.Spec.RoleKeys,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("updating user grant: %w", err)
		}
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.GrantId = grantID
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	cr.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Synced",
			Message:            fmt.Sprintf("UserGrant %q synced successfully", cr.Name),
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("usergrant reconciled", "grantId", grantID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *UserGrantReconciler) resolveProjectID(ctx context.Context, projectName string) (string, error) {
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
func (r *UserGrantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.UserGrant{}).
		Named("usergrant").
		Complete(r)
}
