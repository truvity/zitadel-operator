package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/truvity/zitadel-operator/internal/config"
)

// fieldManagerFor returns the SSA field manager identity for status writes.
// Each operator process owns its own status fields via a per-instance manager
// (zitadel-operator/<instance identity>), enabling two operators to co-write
// status on dual-served CRs without wiping each other's conditions. The
// identity is the config's instanceAlias (falling back to the domain) so a
// domain migration does not change field ownership.
func fieldManagerFor(cfg *config.Config) string {
	if cfg != nil && cfg.InstanceIdentity() != "" {
		return "zitadel-operator/" + cfg.InstanceIdentity()
	}
	return "zitadel-operator"
}

// applyStatus persists the object's status via Server-Side Apply with the
// operator's per-instance field manager. Unlike read-modify-write
// Status().Update, SSA merges per manager: conditions are a listType=map
// keyed by type, so conditions written by another manager survive.
// This is the v0.18 hard prerequisite (see DESIGN "SSA Status Discipline"):
// full-object Update was demonstrated to silently wipe in-memory condition
// edits and cannot support two writers at all.
func applyStatus(ctx context.Context, k8s client.Client, cfg *config.Config, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, k8s.Scheme())
	if err != nil {
		return fmt.Errorf("resolving GVK for SSA status apply: %w", err)
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("converting object for SSA status apply: %w", err)
	}
	patch := &unstructured.Unstructured{Object: map[string]interface{}{}}
	patch.SetGroupVersionKind(gvk)
	patch.SetName(obj.GetName())
	patch.SetNamespace(obj.GetNamespace())
	if status, ok := m["status"]; ok {
		patch.Object["status"] = status
	} else {
		patch.Object["status"] = map[string]interface{}{}
	}
	return k8s.Status().Patch(ctx, patch, client.Apply, //nolint:staticcheck // SA1019: unstructured apply-patch is the stable path for a minimal status-only SSA document; the typed Apply() API needs generated apply configurations
		client.FieldOwner(fieldManagerFor(cfg)), client.ForceOwnership)
}

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
func markReady(ctx context.Context, k8s client.Client, cfg *config.Config, obj client.Object, sf statusFields, statusChanged bool) error {
	if *sf.ready && !statusChanged {
		return nil
	}
	now := metav1.NewTime(time.Now())
	*sf.ready = true
	if sf.lastSyncTime != nil {
		*sf.lastSyncTime = &now
	}
	setCondition(sf.conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
	return applyStatus(ctx, k8s, cfg, obj)
}

// checkBindingLevel enforces the INF-424 degradation matrix: instance-level
// resources (Default* policies, instance IdPs, providers, actions, org
// creation) are not reconcilable under an org-owner binding. The CR gets a
// Ready=False / NotSupportedAtBindingLevel condition and requeues slowly.
// Callers must skip this check during deletion so finalizers never deadlock.
func checkBindingLevel(ctx context.Context, k8s client.Client, cfg *config.Config, obj client.Object, conditions *[]metav1.Condition, ready *bool) (done bool, result ctrl.Result, err error) {
	if cfg == nil || cfg.Binding != config.BindingOrgOwner || !obj.GetDeletionTimestamp().IsZero() {
		return false, ctrl.Result{}, nil
	}
	if ready != nil {
		*ready = false
	}
	setCondition(conditions, ConditionTypeReady, metav1.ConditionFalse, "NotSupportedAtBindingLevel",
		"this resource requires an iam-owner binding; the operator is bound as org-owner (org "+cfg.BoundOrganizationId+")")
	if err := applyStatus(ctx, k8s, cfg, obj); err != nil {
		return true, ctrl.Result{}, err
	}
	log.FromContext(ctx).Info("instance-level resource not supported at org-owner binding", "kind", obj.GetObjectKind().GroupVersionKind().Kind)
	return true, ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// waitForRef checks if err is a "ref not ready" condition and, if so, sets
// a condition on the object and returns a requeue result. Returns (true, result) if
// handled, (false, _) if the error is not a ref-not-ready error.
func waitForRef(ctx context.Context, k8s client.Client, cfg *config.Config, obj client.Object, conditions *[]metav1.Condition, conditionReason string, err error) (waiting bool, result ctrl.Result) {
	if !isRefNotReady(err) {
		return false, ctrl.Result{}
	}
	logger := log.FromContext(ctx)
	logger.Info("waiting for ref to become ready", "reason", conditionReason, "error", err)
	setCondition(conditions, ConditionTypeReady, metav1.ConditionFalse, conditionReason, err.Error())
	_ = applyStatus(ctx, k8s, cfg, obj)
	return true, ctrl.Result{RequeueAfter: requeueOnError}
}
