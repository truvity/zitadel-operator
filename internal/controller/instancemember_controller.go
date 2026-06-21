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
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
)

// InstanceMemberReconciler reconciles an InstanceMember object.
type InstanceMemberReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=instancemembers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=instancemembers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=instancemembers/finalizers,verbs=update

func (r *InstanceMemberReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.InstanceMember
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve user ID.
	userID, err := resolveUserIdIncludingHuman(ctx, r.Client, cr.Spec.UserRef, cr.Spec.UserId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for user ref to become ready", "error", err)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving user: %w", err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		_, err := r.Zitadel.Admin().RemoveIAMMember(ctx, &admin.RemoveIAMMemberRequest{
			UserId: userID,
		})
		if err != nil && status.Code(err) != codes.NotFound {
			return ctrl.Result{}, fmt.Errorf("removing IAM member: %w", err)
		}
		if removeFinalizer(&cr) {
			if err := r.Update(ctx, &cr); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer.
	if addFinalizer(&cr) {
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Ensure instance member exists — try update first, then add.
	_, err = r.Zitadel.Admin().UpdateIAMMember(ctx, &admin.UpdateIAMMemberRequest{
		UserId: userID,
		Roles:  cr.Spec.Roles,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Member doesn't exist, add it.
			_, err = r.Zitadel.Admin().AddIAMMember(ctx, &admin.AddIAMMemberRequest{
				UserId: userID,
				Roles:  cr.Spec.Roles,
			})
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("adding IAM member: %w", err)
			}
		} else {
			return ctrl.Result{}, fmt.Errorf("updating IAM member: %w", err)
		}
	}

	// Update status.
	if !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("instancemember reconciled", "userId", userID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstanceMemberReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.InstanceMember{}).
		Named("instancemember").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
