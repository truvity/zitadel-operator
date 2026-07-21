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
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	actionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/action/v2"
)

// ActionTargetReconciler reconciles an ActionTarget object.
type ActionTargetReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
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

	// INF-424 degradation matrix: instance-level resources are not supported
	// under an org-owner binding (no-op during deletion so finalizers complete).
	if done, result, err := checkBindingLevel(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, &cr.Status.Ready); done {
		return result, err
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
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		cr.Status.LastSyncTime = &now
		if err := applyStatus(ctx, r.Client, r.Config, &cr); err != nil {
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

	createReq := &actionv2.CreateTargetRequest{
		Name:        displayName,
		Timeout:     durationpb.New(timeout),
		Endpoint:    cr.Spec.Endpoint,
		PayloadType: mapPayloadType(cr.Spec.PayloadType),
	}
	setCreateTargetType(createReq, cr)

	createResp, err := r.Zitadel.Action().CreateTarget(ctx, createReq)
	if err != nil {
		return "", fmt.Errorf("creating target: %w", err)
	}

	return createResp.GetId(), nil
}

func (r *ActionTargetReconciler) updateTarget(ctx context.Context, targetID string, cr *zitadelv1alpha2.ActionTarget) error {
	timeout := parseDuration(cr.Spec.Timeout, 10*time.Second)

	updateReq := &actionv2.UpdateTargetRequest{
		Id:          targetID,
		Name:        strPtr(cr.DisplayName()),
		Timeout:     durationpb.New(timeout),
		Endpoint:    strPtr(cr.Spec.Endpoint),
		PayloadType: mapPayloadType(cr.Spec.PayloadType),
	}
	setUpdateTargetType(updateReq, cr)

	_, err := r.Zitadel.Action().UpdateTarget(ctx, updateReq)
	if err != nil {
		return fmt.Errorf("updating target: %w", err)
	}
	return nil
}

// setCreateTargetType sets the target type oneof on a CreateTargetRequest based on the CRD spec.
func setCreateTargetType(req *actionv2.CreateTargetRequest, cr *zitadelv1alpha2.ActionTarget) {
	switch cr.Spec.TargetType {
	case zitadelv1alpha2.ActionTargetTypeRestWebhook:
		req.TargetType = &actionv2.CreateTargetRequest_RestWebhook{
			RestWebhook: &actionv2.RESTWebhook{
				InterruptOnError: cr.Spec.InterruptOnError,
			},
		}
	case zitadelv1alpha2.ActionTargetTypeRestAsync:
		req.TargetType = &actionv2.CreateTargetRequest_RestAsync{
			RestAsync: &actionv2.RESTAsync{},
		}
	default: // restCall (default)
		req.TargetType = &actionv2.CreateTargetRequest_RestCall{
			RestCall: &actionv2.RESTCall{
				InterruptOnError: cr.Spec.InterruptOnError,
			},
		}
	}
}

// setUpdateTargetType sets the target type oneof on an UpdateTargetRequest based on the CRD spec.
func setUpdateTargetType(req *actionv2.UpdateTargetRequest, cr *zitadelv1alpha2.ActionTarget) {
	switch cr.Spec.TargetType {
	case zitadelv1alpha2.ActionTargetTypeRestWebhook:
		req.TargetType = &actionv2.UpdateTargetRequest_RestWebhook{
			RestWebhook: &actionv2.RESTWebhook{
				InterruptOnError: cr.Spec.InterruptOnError,
			},
		}
	case zitadelv1alpha2.ActionTargetTypeRestAsync:
		req.TargetType = &actionv2.UpdateTargetRequest_RestAsync{
			RestAsync: &actionv2.RESTAsync{},
		}
	default: // restCall (default)
		req.TargetType = &actionv2.UpdateTargetRequest_RestCall{
			RestCall: &actionv2.RESTCall{
				InterruptOnError: cr.Spec.InterruptOnError,
			},
		}
	}
}

// mapPayloadType maps the CRD payloadType enum to the SDK's PayloadType enum.
func mapPayloadType(pt zitadelv1alpha2.ActionTargetPayloadType) actionv2.PayloadType {
	switch pt {
	case zitadelv1alpha2.ActionTargetPayloadTypeJWT:
		return actionv2.PayloadType_PAYLOAD_TYPE_JWT
	case zitadelv1alpha2.ActionTargetPayloadTypeJWE:
		return actionv2.PayloadType_PAYLOAD_TYPE_JWE
	default: // json (default) or empty
		return actionv2.PayloadType_PAYLOAD_TYPE_JSON
	}
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
