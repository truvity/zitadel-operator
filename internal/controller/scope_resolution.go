package controller

import (
	"context"
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"
)

const (
	// ConditionTypeScopeResolved reports namespace->scope resolution state
	// (v0.18 scope maps). False = fail-closed; the reason distinguishes
	// MapsNotSynced (transient) from NoMatchingRule / ScopeConflict /
	// InstanceMismatch (steady-state rejects).
	ConditionTypeScopeResolved = "ScopeResolved"

	// requeueOnScopeSync is the short requeue while informers sync.
	requeueOnScopeSync = 2 * time.Second
)

// resolvedScope bundles the outcome of scope resolution: the Zitadel client
// to reconcile with, and (when scope maps are active) the scope + delegate.
type resolvedScope struct {
	// zc is the client to use: the delegated per-scope client when scoped,
	// the binding client in passthrough mode.
	zc       *zitadel.Client
	scope    *scopemap.Scope
	delegate *delegation.Delegate
}

// resolveScopeAndClient resolves the namespace scope.
//
// done=true means the caller must return (result, err) immediately
// (fail-closed or waiting for informers).
//
// While the object is being deleted, resolution/delegation failures fall back
// to the binding client so finalizers can complete (prototype decision;
// recorded in the findings).
func resolveScopeAndClient(
	ctx context.Context,
	k8s client.Client,
	resolver *scopemap.Resolver,
	delegator *delegation.Manager,
	binding *zitadel.Client,
	obj client.Object,
	conditions *[]metav1.Condition,
	namespace string,
	deleting bool,
) (rs resolvedScope, done bool, result ctrl.Result, err error) {
	if resolver == nil {
		return resolvedScope{zc: binding}, false, ctrl.Result{}, nil
	}
	logger := log.FromContext(ctx)

	s, rerr := resolver.Resolve(ctx, namespace)
	if rerr != nil {
		if deleting {
			logger.Info("scope resolution failed during deletion, falling back to binding client", "error", rerr)
			return resolvedScope{zc: binding}, false, ctrl.Result{}, nil
		}
		reason, requeue := classifyScopeError(rerr)
		if reason == "" {
			return resolvedScope{}, true, ctrl.Result{}, rerr
		}
		setCondition(conditions, ConditionTypeScopeResolved, metav1.ConditionFalse, reason, rerr.Error())
		_ = k8s.Status().Update(ctx, obj)
		logger.Info("scope resolution fail-closed", "reason", reason, "namespace", namespace, "error", rerr)
		return resolvedScope{}, true, ctrl.Result{RequeueAfter: requeue}, nil
	}

	if s == nil {
		// Passthrough: no scope maps exist, feature off.
		return resolvedScope{zc: binding}, false, ctrl.Result{}, nil
	}

	d, derr := delegator.Ensure(ctx, s)
	if derr != nil {
		if deleting {
			logger.Info("delegation failed during deletion, falling back to binding client", "error", derr)
			return resolvedScope{zc: binding, scope: s}, false, ctrl.Result{}, nil
		}
		setCondition(conditions, ConditionTypeScopeResolved, metav1.ConditionFalse, "DelegationFailed", derr.Error())
		_ = k8s.Status().Update(ctx, obj)
		return resolvedScope{}, true, ctrl.Result{}, derr
	}

	setCondition(conditions, ConditionTypeScopeResolved, metav1.ConditionTrue, "Resolved",
		"scope "+s.MapName+"/"+s.RuleName+" (org "+s.OrganizationID+", delegate "+d.UserID+")")
	return resolvedScope{zc: d.Client, scope: s, delegate: d}, false, ctrl.Result{}, nil
}

// applyScopeResolvedCondition (re-)sets the ScopeResolved=True condition for a
// successfully delegated scope. Needed after any full-object Update (e.g.
// ensureFinalizer), which refreshes the object from the server and discards
// in-memory condition edits.
func applyScopeResolvedCondition(rs resolvedScope, conditions *[]metav1.Condition) {
	if rs.scope == nil || rs.delegate == nil {
		return
	}
	setCondition(conditions, ConditionTypeScopeResolved, metav1.ConditionTrue, "Resolved",
		"scope "+rs.scope.MapName+"/"+rs.scope.RuleName+" (org "+rs.scope.OrganizationID+", delegate "+rs.delegate.UserID+")")
}

// classifyScopeError maps resolver errors to condition reasons and requeue
// intervals. MapsNotSynced is transient (short requeue); the rest are
// steady-state fail-closed (slower requeue to observe map/namespace changes).
// Empty reason = unclassified, surface as reconcile error.
func classifyScopeError(err error) (string, time.Duration) {
	var noMatch *scopemap.NoMatchError
	var conflict *scopemap.ConflictError
	var mismatch *scopemap.InstanceMismatchError
	var notReady *scopemap.MapNotReadyError
	switch {
	case errors.Is(err, scopemap.ErrMapsNotSynced):
		return "MapsNotSynced", requeueOnScopeSync
	case errors.As(err, &noMatch):
		return "NoMatchingRule", requeueOnError
	case errors.As(err, &conflict):
		return "ScopeConflict", requeueOnError
	case errors.As(err, &mismatch):
		return "InstanceMismatch", requeueOnError
	case errors.As(err, &notReady):
		return "ScopeMapNotReady", requeueOnScopeSync
	default:
		return "", 0
	}
}
