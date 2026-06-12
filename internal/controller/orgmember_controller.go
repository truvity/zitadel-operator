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

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// OrgMemberReconciler reconciles an OrgMember object.
type OrgMemberReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=orgmembers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=orgmembers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=orgmembers/finalizers,verbs=update

func (r *OrgMemberReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha1.OrgMember
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve userEmail → userId.
	userID, err := resolveUserIDByEmail(ctx, r.Zitadel, cr.Spec.UserEmail)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolving user email %q: %w", cr.Spec.UserEmail, err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		_, err := r.Zitadel.Management().RemoveOrgMember(ctx, &management.RemoveOrgMemberRequest{ //nolint:staticcheck // v1 API required
			UserId: userID,
		})
		if err != nil && status.Code(err) != codes.NotFound {
			return ctrl.Result{}, fmt.Errorf("removing org member: %w", err)
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

	// Add or update org member.
	if cr.Status.UserId == "" {
		// First time — add member.
		_, err = r.Zitadel.Management().AddOrgMember(ctx, &management.AddOrgMemberRequest{ //nolint:staticcheck // v1 API required
			UserId: userID,
			Roles:  cr.Spec.Roles,
		})
		if err != nil {
			// If already exists, try update instead.
			if status.Code(err) == codes.AlreadyExists {
				_, err = r.Zitadel.Management().UpdateOrgMember(ctx, &management.UpdateOrgMemberRequest{ //nolint:staticcheck // v1 API required
					UserId: userID,
					Roles:  cr.Spec.Roles,
				})
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("updating org member: %w", err)
				}
			} else {
				return ctrl.Result{}, fmt.Errorf("adding org member: %w", err)
			}
		}
	} else {
		// Update roles.
		_, err = r.Zitadel.Management().UpdateOrgMember(ctx, &management.UpdateOrgMemberRequest{ //nolint:staticcheck // v1 API required
			UserId: userID,
			Roles:  cr.Spec.Roles,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("updating org member: %w", err)
		}
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.UserId = userID
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	cr.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Synced",
			Message:            fmt.Sprintf("OrgMember %q synced (userId=%s)", cr.Spec.UserEmail, userID),
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("orgmember reconciled", "userEmail", cr.Spec.UserEmail, "userId", userID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrgMemberReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.OrgMember{}).
		Named("orgmember").
		WithEventFilter(generationChangedPredicate()).
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
