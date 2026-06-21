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

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// OrgMemberReconciler reconciles an OrgMember object.
type OrgMemberReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=orgmembers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=orgmembers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=orgmembers/finalizers,verbs=update

func (r *OrgMemberReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.OrgMember
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

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

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
		_, err := r.Zitadel.Management().RemoveOrgMember(ctx, &management.RemoveOrgMemberRequest{
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

	// Add finalizer.
	if addFinalizer(&cr) {
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Ensure org member exists — try update first, then add.
	_, err = r.Zitadel.Management().UpdateOrgMember(ctx, &management.UpdateOrgMemberRequest{
		UserId: userID,
		Roles:  cr.Spec.Roles,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Member doesn't exist, add it.
			_, err = r.Zitadel.Management().AddOrgMember(ctx, &management.AddOrgMemberRequest{
				UserId: userID,
				Roles:  cr.Spec.Roles,
			})
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("adding org member: %w", err)
			}
		} else {
			return ctrl.Result{}, fmt.Errorf("updating org member: %w", err)
		}
	}

	// Update status.
	if cr.Status.OrganizationId != orgID || !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.OrganizationId = orgID
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("orgmember reconciled", "userId", userID, "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// resolveUserIdIncludingHuman resolves a user ID from either a UserRef or explicit UserId.
// Unlike resolveUserId in common.go which only checks MachineUser,
// this also checks HumanUser.
func resolveUserIdIncludingHuman(ctx context.Context, k8s client.Client, ref *zitadelv1alpha2.ResourceRef, explicitID, sourceNamespace string) (string, error) {
	if ref != nil && explicitID != "" {
		return "", fmt.Errorf("userRef and userId are mutually exclusive")
	}

	if ref == nil && explicitID == "" {
		return "", fmt.Errorf("one of userRef or userId is required")
	}

	if explicitID != "" {
		return explicitID, nil
	}

	ns := ref.Namespace
	if ns == "" {
		ns = sourceNamespace
	}

	// Try MachineUser first.
	var mu zitadelv1alpha2.MachineUser
	if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, &mu); err == nil {
		if mu.Status.UserId == "" {
			return "", fmt.Errorf("userRef %s/%s not yet ready (no userId in status)", ns, ref.Name)
		}
		return mu.Status.UserId, nil
	}

	// Try HumanUser.
	var hu zitadelv1alpha2.HumanUser
	if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, &hu); err == nil {
		if hu.Status.UserId == "" {
			return "", fmt.Errorf("userRef %s/%s not yet ready (no userId in status)", ns, ref.Name)
		}
		return hu.Status.UserId, nil
	}

	return "", fmt.Errorf("userRef %s/%s not found (tried MachineUser and HumanUser)", ns, ref.Name)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrgMemberReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.OrgMember{}).
		Named("orgmember").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
