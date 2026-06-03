package controller

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
	userv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user"
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
				if err != nil && grpcstatus.Code(err) != codes.NotFound {
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
		// If the user doesn't exist yet (hasn't logged in), set UserPending condition and requeue with backoff.
		if isUserNotFoundError(err) {
			logger.Info("user not found in Zitadel, setting UserPending condition", "email", cr.Spec.UserEmail)
			return r.setUserPendingStatus(ctx, &cr)
		}
		return ctrl.Result{}, fmt.Errorf("resolving user email %q: %w", cr.Spec.UserEmail, err)
	}

	// Resolve projectRef → projectId.
	projectID, err := r.resolveProjectID(ctx, cr.Spec.ProjectRef)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolving project %q: %w", cr.Spec.ProjectRef, err)
	}

	// Create or update grant.
	grantID := cr.Status.GrantId
	if grantID != "" {
		// Update existing grant.
		_, err := r.Zitadel.Management().UpdateUserGrant(ctx, &management.UpdateUserGrantRequest{ //nolint:staticcheck // v1 API required
			GrantId:  grantID,
			UserId:   userID,
			RoleKeys: cr.Spec.RoleKeys,
		})
		if err != nil {
			switch grpcstatus.Code(err) {
			case codes.FailedPrecondition:
				// Grant hasn't changed — treat as success.
			case codes.NotFound:
				// Grant was deleted externally — clear stored grantId and fall through to add path.
				logger.Info("stored grant not found, re-creating", "grantId", grantID)
				grantID = ""
			default:
				return ctrl.Result{}, fmt.Errorf("updating user grant: %w", err)
			}
		}
	}

	if grantID == "" {
		// Create new user grant.
		resp, addErr := r.Zitadel.Management().AddUserGrant(ctx, &management.AddUserGrantRequest{ //nolint:staticcheck // v1 API required
			UserId:    userID,
			ProjectId: projectID,
			RoleKeys:  cr.Spec.RoleKeys,
		})
		if addErr != nil {
			if grpcstatus.Code(addErr) == codes.AlreadyExists {
				// Grant already exists for this user+project — look it up and use the existing one.
				existingID, roleKeys, lookupErr := r.lookupExistingGrant(ctx, userID, projectID)
				if lookupErr != nil {
					return ctrl.Result{}, fmt.Errorf("looking up existing grant for user %q project %q: %w", userID, projectID, lookupErr)
				}
				grantID = existingID
				// Update roleKeys if they differ from desired.
				if !roleKeysEqual(roleKeys, cr.Spec.RoleKeys) {
					_, updateErr := r.Zitadel.Management().UpdateUserGrant(ctx, &management.UpdateUserGrantRequest{ //nolint:staticcheck // v1 API required
						GrantId:  grantID,
						UserId:   userID,
						RoleKeys: cr.Spec.RoleKeys,
					})
					if updateErr != nil && grpcstatus.Code(updateErr) != codes.FailedPrecondition {
						return ctrl.Result{}, fmt.Errorf("updating existing user grant %q: %w", grantID, updateErr)
					}
				}
			} else {
				return ctrl.Result{}, fmt.Errorf("adding user grant: %w", addErr)
			}
		} else {
			grantID = resp.GetUserGrantId()
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

// lookupExistingGrant finds an existing user grant by userId and projectId.
func (r *UserGrantReconciler) lookupExistingGrant(ctx context.Context, userID, projectID string) (grantID string, roleKeys []string, err error) {
	resp, err := r.Zitadel.Management().ListUserGrants(ctx, &management.ListUserGrantRequest{ //nolint:staticcheck // v1 API required
		Queries: []*userv1.UserGrantQuery{
			{
				Query: &userv1.UserGrantQuery_UserIdQuery{
					UserIdQuery: &userv1.UserGrantUserIDQuery{
						UserId: userID,
					},
				},
			},
			{
				Query: &userv1.UserGrantQuery_ProjectIdQuery{
					ProjectIdQuery: &userv1.UserGrantProjectIDQuery{
						ProjectId: projectID,
					},
				},
			},
		},
	})
	if err != nil {
		return "", nil, fmt.Errorf("listing user grants: %w", err)
	}

	for _, g := range resp.GetResult() {
		if g.GetUserId() == userID && g.GetProjectId() == projectID {
			return g.GetId(), g.GetRoleKeys(), nil
		}
	}

	return "", nil, fmt.Errorf("grant not found for user %q project %q", userID, projectID)
}

// setUserPendingStatus sets a UserPending condition and requeues after requeueInterval (5 minutes).
func (r *UserGrantReconciler) setUserPendingStatus(ctx context.Context, cr *zitadelv1alpha1.UserGrant) (ctrl.Result, error) {
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

// isUserNotFoundError checks if the error indicates the user was not found in Zitadel.
func isUserNotFoundError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

// roleKeysEqual checks if two role key slices contain the same elements (order-independent).
func roleKeysEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sortedA := make([]string, len(a))
	copy(sortedA, a)
	slices.Sort(sortedA)
	sortedB := make([]string, len(b))
	copy(sortedB, b)
	slices.Sort(sortedB)
	return slices.Equal(sortedA, sortedB)
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserGrantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.UserGrant{}).
		Named("usergrant").
		Complete(r)
}
