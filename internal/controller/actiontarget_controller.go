package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	actionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/action/v2"
)

// ActionTargetReconciler reconciles an ActionTarget object.
type ActionTargetReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=actiontargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=actiontargets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=actiontargets/finalizers,verbs=update

func (r *ActionTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.ActionTarget
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.TargetId != "" {
			_, err := r.Zitadel.Action().DeleteTarget(ctx, &actionv2.DeleteTargetRequest{
				Id: cr.Status.TargetId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("deleting target: %w", err)
			}
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

	// Ensure target exists.
	targetID, err := r.ensureTarget(ctx, &cr)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	if cr.Status.TargetId != targetID || !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.TargetId = targetID
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("actiontarget reconciled", "targetId", targetID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ActionTargetReconciler) ensureTarget(ctx context.Context, cr *zitadelv1alpha2.ActionTarget) (string, error) {
	// If we have a target ID, verify and update it.
	if cr.Status.TargetId != "" {
		_, err := r.Zitadel.Action().GetTarget(ctx, &actionv2.GetTargetRequest{
			Id: cr.Status.TargetId,
		})
		if err == nil {
			// Target exists, update it.
			if err := r.updateTarget(ctx, cr.Status.TargetId, cr); err != nil {
				return "", err
			}
			return cr.Status.TargetId, nil
		}
		if status.Code(err) != codes.NotFound {
			return "", fmt.Errorf("getting target: %w", err)
		}
		// Target deleted externally, recreate.
	}

	// Search by name.
	listResp, err := r.Zitadel.Action().ListTargets(ctx, &actionv2.ListTargetsRequest{})
	if err != nil {
		return "", fmt.Errorf("listing targets: %w", err)
	}

	displayName := cr.DisplayName()
	for _, t := range listResp.GetTargets() {
		if t.GetName() == displayName {
			// Found existing, update and return.
			if err := r.updateTarget(ctx, t.GetId(), cr); err != nil {
				return "", err
			}
			return t.GetId(), nil
		}
	}

	// Create new target.
	timeout := parseDuration(cr.Spec.Timeout, 10*time.Second)

	createResp, err := r.Zitadel.Action().CreateTarget(ctx, &actionv2.CreateTargetRequest{
		Name: displayName,
		TargetType: &actionv2.CreateTargetRequest_RestCall{
			RestCall: &actionv2.RESTCall{
				InterruptOnError: cr.Spec.InterruptOnError,
			},
		},
		Timeout:  durationpb.New(timeout),
		Endpoint: cr.Spec.Endpoint,
	})
	if err != nil {
		return "", fmt.Errorf("creating target: %w", err)
	}

	return createResp.GetId(), nil
}

func (r *ActionTargetReconciler) updateTarget(ctx context.Context, targetID string, cr *zitadelv1alpha2.ActionTarget) error {
	timeout := parseDuration(cr.Spec.Timeout, 10*time.Second)

	_, err := r.Zitadel.Action().UpdateTarget(ctx, &actionv2.UpdateTargetRequest{
		Id:   targetID,
		Name: strPtr(cr.DisplayName()),
		TargetType: &actionv2.UpdateTargetRequest_RestCall{
			RestCall: &actionv2.RESTCall{
				InterruptOnError: cr.Spec.InterruptOnError,
			},
		},
		Timeout:  durationpb.New(timeout),
		Endpoint: strPtr(cr.Spec.Endpoint),
	})
	if err != nil {
		return fmt.Errorf("updating target: %w", err)
	}
	return nil
}

func parseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

func strPtr(s string) *string {
	return &s
}

// SetupWithManager sets up the controller with the Manager.
func (r *ActionTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.ActionTarget{}).
		Named("actiontarget").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
