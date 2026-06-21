package controller

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/truvity/zitadel-operator/internal/config"
)

// handleDeletion checks if the object is being deleted. If so, it calls the provided
// deleteFunc (which may be nil for leave-as-is behavior), removes the finalizer, and
// returns done=true. The caller should return immediately when done=true.
func handleDeletion(ctx context.Context, k8s client.Client, obj client.Object, deleteFunc func() error) (done bool, result ctrl.Result, err error) { //nolint:unparam // result kept for caller pattern consistency
	if obj.GetDeletionTimestamp().IsZero() {
		return false, ctrl.Result{}, nil
	}
	if deleteFunc != nil {
		if delErr := deleteFunc(); delErr != nil {
			logger := log.FromContext(ctx)
			logger.Info("cleanup during deletion returned error (non-blocking)", "error", delErr)
		}
	}
	if removeFinalizer(obj) {
		if err := k8s.Update(ctx, obj); err != nil {
			return true, ctrl.Result{}, err
		}
	}
	return true, ctrl.Result{}, nil
}

// handleDeletionStrict checks if the object is being deleted. If so, it calls the provided
// deleteFunc and returns a hard error if it fails (blocking deletion until cleanup succeeds).
// This variant is used when the external resource MUST be cleaned up before the finalizer is removed.
func handleDeletionStrict(ctx context.Context, k8s client.Client, obj client.Object, deleteFunc func() error) (done bool, result ctrl.Result, err error) { //nolint:unparam // result kept for caller pattern consistency
	if obj.GetDeletionTimestamp().IsZero() {
		return false, ctrl.Result{}, nil
	}
	if deleteFunc != nil {
		if delErr := deleteFunc(); delErr != nil {
			return true, ctrl.Result{}, delErr
		}
	}
	if removeFinalizer(obj) {
		if err := k8s.Update(ctx, obj); err != nil {
			return true, ctrl.Result{}, err
		}
	}
	return true, ctrl.Result{}, nil
}

// handleSingletonDeletion handles deletion for instance-default singleton policies.
// It resets to defaults only if the reset-on-delete annotation is present.
// The resetFunc is called only when shouldResetOnDelete returns true.
func handleSingletonDeletion(ctx context.Context, k8s client.Client, obj client.Object, resetFunc func()) (done bool, result ctrl.Result, err error) { //nolint:unparam // result kept for caller pattern consistency
	if obj.GetDeletionTimestamp().IsZero() {
		return false, ctrl.Result{}, nil
	}
	if shouldResetOnDelete(obj) && resetFunc != nil {
		resetFunc()
	}
	if removeFinalizer(obj) {
		if err := k8s.Update(ctx, obj); err != nil {
			return true, ctrl.Result{}, err
		}
	}
	return true, ctrl.Result{}, nil
}

// ensureFinalizer adds the finalizer if not present and persists the change.
// Returns an error if the update fails. The caller should always continue
// reconciling after this call (the finalizer addition does not bump generation,
// so no re-reconcile will be triggered automatically).
func ensureFinalizer(ctx context.Context, k8s client.Client, obj client.Object) error {
	if addFinalizer(obj) {
		return k8s.Update(ctx, obj)
	}
	return nil
}

// statusFields groups the mutable status fields that lifecycle helpers update.
type statusFields struct {
	conditions   *[]metav1.Condition
	ready        *bool
	lastSyncTime **metav1.Time
}

// markReady sets the resource to Ready with a Reconciled condition if not already ready.
// The statusChanged parameter allows callers to force an update even when ready is already true
// (e.g. when a status field like OrganizationId changed).
func markReady(ctx context.Context, k8s client.Client, obj client.Object, sf statusFields, statusChanged bool) error {
	if *sf.ready && !statusChanged {
		return nil
	}
	now := metav1.NewTime(time.Now())
	*sf.ready = true
	if sf.lastSyncTime != nil {
		*sf.lastSyncTime = &now
	}
	setCondition(sf.conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
	return k8s.Status().Update(ctx, obj)
}

// waitForRef checks if err is a "ref not ready" condition and, if so, sets
// a condition on the object and returns a requeue result. Returns (true, result) if
// handled, (false, _) if the error is not a ref-not-ready error.
func waitForRef(ctx context.Context, k8s client.Client, obj client.Object, conditions *[]metav1.Condition, conditionReason string, err error) (waiting bool, result ctrl.Result) {
	if !isRefNotReady(err) {
		return false, ctrl.Result{}
	}
	logger := log.FromContext(ctx)
	logger.Info("waiting for ref to become ready", "reason", conditionReason, "error", err)
	setCondition(conditions, ConditionTypeReady, metav1.ConditionFalse, conditionReason, err.Error())
	_ = k8s.Status().Update(ctx, obj)
	return true, ctrl.Result{RequeueAfter: requeueOnError}
}

// checkProjectScope validates project scope and returns a requeue result if validation fails.
// Returns (true, result, err) if the caller should return immediately (validation failed or error).
func checkProjectScope(ctx context.Context, k8s client.Client, cfg *config.Config, namespace string, obj client.Object, conditions *[]metav1.Condition) (done bool, result ctrl.Result, err error) {
	shouldProceed, err := validateProjectScope(ctx, k8s, cfg, namespace, conditions)
	if err != nil {
		return true, ctrl.Result{}, err
	}
	if !shouldProceed {
		_ = k8s.Status().Update(ctx, obj)
		log.FromContext(ctx).Info("project scope validation failed, requeueing",
			"namespace", namespace,
			"label", cfg.ProjectScopeLabel)
		return true, ctrl.Result{RequeueAfter: requeueOnError}, nil
	}
	return false, ctrl.Result{}, nil
}
