package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
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

// zclientCtxKey carries a per-reconcile Zitadel client override (the
// delegated per-scope client) through the call chain without threading a
// parameter into every controller helper.
type zclientCtxKey struct{}

// withZClient returns a context carrying the given Zitadel client override.
func withZClient(ctx context.Context, zc *zitadel.Client) context.Context {
	if zc == nil {
		return ctx
	}
	return context.WithValue(ctx, zclientCtxKey{}, zc)
}

// zclient returns the per-reconcile Zitadel client: the delegated client when
// scope resolution put one in the context, otherwise the fallback (the
// reconciler's binding client).
func zclient(ctx context.Context, fallback *zitadel.Client) *zitadel.Client {
	if zc, ok := ctx.Value(zclientCtxKey{}).(*zitadel.Client); ok && zc != nil {
		return zc
	}
	return fallback
}

// ConditionTypeInstanceResolved reports dual-serving instance affinity
// (v0.18). False/AmbiguousInstance = the namespace is served by more than one
// operator and spec.instance is unset — every operator fails closed.
const ConditionTypeInstanceResolved = "InstanceResolved"

// instanceGate implements the v0.18 dual-serving contract for tenant CRs:
//
//   - spec.instance pinned to a foreign domain: the CR belongs to another
//     operator — completely untouched (no finalizer, no status, no API calls).
//   - spec.instance pinned to this operator: reconciled normally.
//   - spec.instance unset: this operator SSA-writes its presence, then checks
//     the CR's managedFields for another zitadel-operator/* field manager. If
//     one exists the namespace is dual-served: fail closed with a
//     deterministic AmbiguousInstance condition (identical from every
//     operator, so co-ownership of the condition converges instead of
//     flapping).
//
// Callers skip the gate during deletion so finalizer cleanup always proceeds
// (foreign-instance deletes are no-ops server-side: the recorded IDs don't
// exist on the other instance).
func instanceGate(ctx context.Context, k8s client.Client, cfg *config.Config, obj client.Object, pinned string, conditions *[]metav1.Condition) (done bool, result ctrl.Result, err error) {
	identity := ""
	if cfg != nil {
		identity = cfg.InstanceIdentity()
	}
	if pinned != "" {
		if pinned != identity {
			// Foreign pin: hands-off entirely.
			return true, ctrl.Result{}, nil
		}
		setCondition(conditions, ConditionTypeInstanceResolved, metav1.ConditionTrue, "Pinned",
			"spec.instance pins this resource to instance "+identity)
		return false, ctrl.Result{}, nil
	}

	mine := fieldManagerFor(cfg)
	if !foreignOperatorManagerPresent(obj, mine) {
		// Announce presence via SSA before any external action so a second
		// operator serving this namespace can detect the dual-serve.
		if !hasCondition(*conditions, ConditionTypeInstanceResolved, metav1.ConditionTrue, "Assumed") {
			setCondition(conditions, ConditionTypeInstanceResolved, metav1.ConditionTrue, "Assumed",
				"no other operator has written status; serving without an explicit spec.instance pin")
			if err := applyStatus(ctx, k8s, cfg, obj); err != nil {
				return true, ctrl.Result{}, err
			}
		}
		return false, ctrl.Result{}, nil
	}

	// Dual-served and unset: fail closed. The message is deterministic and
	// identical from every operator so shared ownership converges.
	setCondition(conditions, ConditionTypeInstanceResolved, metav1.ConditionFalse, "AmbiguousInstance",
		"namespace is served by multiple zitadel operators; set spec.instance to pin this resource to one instance")
	if err := applyStatus(ctx, k8s, cfg, obj); err != nil {
		return true, ctrl.Result{}, err
	}
	log.FromContext(ctx).Info("dual-served namespace with unset spec.instance, fail-closed (AmbiguousInstance)")
	return true, ctrl.Result{RequeueAfter: requeueOnError}, nil
}

// hasCondition reports whether a condition with the given type/status/reason
// is already recorded.
func hasCondition(conditions []metav1.Condition, condType string, condStatus metav1.ConditionStatus, reason string) bool {
	for _, c := range conditions {
		if c.Type == condType && c.Status == condStatus && c.Reason == reason {
			return true
		}
	}
	return false
}

// foreignOperatorManagerPresent scans managedFields for a status write by a
// different zitadel-operator/* field manager.
func foreignOperatorManagerPresent(obj client.Object, mine string) bool {
	for _, mf := range obj.GetManagedFields() {
		if mf.Manager != mine && strings.HasPrefix(mf.Manager, "zitadel-operator") {
			return true
		}
	}
	return false
}

// tenantPreamble runs the entry checks every tenant reconciler shares: the
// v0.18 dual-serving instance gate (skipped during deletion), the v0.19
// ForeignManager management gate (active during deletion — a same-instance
// foreign delete is not a server-side no-op), and namespace scope resolution.
// On success the returned context carries the per-scope Zitadel client for
// zclient().
func tenantPreamble(
	ctx context.Context,
	k8s client.Client,
	cfg *config.Config,
	resolver *scopemap.Resolver,
	delegator *delegation.Manager,
	binding *zitadel.Client,
	obj client.Object,
	pinned string,
	conditions *[]metav1.Condition,
	namespace string,
) (rctx context.Context, rs resolvedScope, done bool, result ctrl.Result, err error) {
	deleting := !obj.GetDeletionTimestamp().IsZero()
	if !deleting {
		if done, result, err := instanceGate(ctx, k8s, cfg, obj, pinned, conditions); done {
			return ctx, resolvedScope{}, true, result, err
		}
	}
	// v0.19: the management gate runs after the instance gate so CRs pinned
	// to a foreign instance stay completely untouched (no condition writes),
	// but before any external action so a same-instance foreign operator
	// never reconciles — or deletes — another deployment's CR.
	if done, result, err := managementGate(ctx, k8s, cfg, obj, conditions); done {
		return ctx, resolvedScope{}, true, result, err
	}
	rs, done, result, err = resolveScopeAndClient(ctx, k8s, cfg, resolver, delegator, binding, obj, conditions, namespace, deleting)
	if done {
		return ctx, resolvedScope{}, true, result, err
	}
	return withZClient(ctx, rs.zc), rs, false, ctrl.Result{}, nil
}

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
// to the binding client so finalizers can complete (a delegate must outlive
// the tenant CRs it serves, but a misordered teardown must not deadlock).
func resolveScopeAndClient(
	ctx context.Context,
	k8s client.Client,
	cfg *config.Config,
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
		_ = applyStatus(ctx, k8s, cfg, obj)
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
		_ = applyStatus(ctx, k8s, cfg, obj)
		if code := status.Code(derr); code == codes.PermissionDenied || code == codes.Unauthenticated {
			// Permission-shaped failures are configuration states (e.g. an
			// org-owner binding that cannot grant), not transient controller
			// errors: fail closed without polluting the error metrics and
			// re-check on the periodic interval.
			logger.Info("delegation denied, fail-closed", "namespace", namespace, "error", derr)
			return resolvedScope{}, true, ctrl.Result{RequeueAfter: requeueInterval}, nil
		}
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
// intervals. MapsNotSynced / ScopeMapNotReady are transient (short requeue);
// NoMatchingRule / ScopeConflict / InstanceMismatch are confirmed
// steady-state rejects and back off to the periodic interval — a namespace
// full of orphaned CRs must not hot-loop every 10s forever. (Spec changes
// still trigger immediate reconciles; recovery after a map fix is picked up
// within one periodic cycle.)
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
		return "NoMatchingRule", requeueInterval
	case errors.As(err, &conflict):
		return "ScopeConflict", requeueInterval
	case errors.As(err, &mismatch):
		return "InstanceMismatch", requeueInterval
	case errors.As(err, &notReady):
		return "ScopeMapNotReady", requeueOnScopeSync
	default:
		return "", 0
	}
}

// resolveScopedOrganizationId resolves the organization for an org-scoped
// tenant CR, honoring the resolved scope:
//   - scoped + CR omits org        -> the scope's org (scope-defaulted)
//   - scoped + CR pins a different org -> OrganizationOutOfScope error
//   - passthrough                  -> legacy resolution (ref or explicit ID)
func resolveScopedOrganizationId(ctx context.Context, k8s client.Client, rs resolvedScope, ref *zitadelv1alpha2.ResourceRef, explicitID, sourceNamespace string) (string, error) {
	if rs.scope == nil {
		return resolveOrganizationId(ctx, k8s, ref, explicitID, sourceNamespace)
	}
	if ref == nil && explicitID == "" {
		return rs.scope.OrganizationID, nil
	}
	orgID, err := resolveOrganizationId(ctx, k8s, ref, explicitID, sourceNamespace)
	if err != nil {
		return "", err
	}
	if orgID != rs.scope.OrganizationID {
		return "", &orgOutOfScopeError{Requested: orgID, ScopeOrg: rs.scope.OrganizationID, MapName: rs.scope.MapName}
	}
	return orgID, nil
}

// orgOutOfScopeError signals a tenant CR pinned an org outside its namespace's
// resolved scope (fail-closed).
type orgOutOfScopeError struct {
	Requested string
	ScopeOrg  string
	MapName   string
}

func (e *orgOutOfScopeError) Error() string {
	return fmt.Sprintf("organization %s is outside the namespace's resolved scope (map %q pins org %s); fail-closed",
		e.Requested, e.MapName, e.ScopeOrg)
}

// resolveScopedProjectId resolves the project for a project-level tenant CR
// (apps, keys, grants, members), honoring the resolved scope:
//   - CR names a project (ref or ID)          -> legacy resolution
//   - project-scoped namespace, CR omits it   -> the scope's project
//   - status already records a project        -> reuse (covers deletion after
//     maps/delegates are gone, when resolution falls back to the binding client)
func resolveScopedProjectId(ctx context.Context, k8s client.Client, rs resolvedScope, ref *zitadelv1alpha2.ResourceRef, explicitID, sourceNamespace, statusProjectID, statusOrgID string) (projectID, orgID string, err error) {
	explicit := ref != nil || explicitID != ""
	switch {
	case !explicit && rs.scope != nil && rs.delegate != nil && rs.delegate.ProjectID != "":
		return rs.delegate.ProjectID, rs.scope.OrganizationID, nil
	case !explicit && statusProjectID != "":
		return statusProjectID, statusOrgID, nil
	default:
		return resolveProjectId(ctx, k8s, ref, explicitID, sourceNamespace)
	}
}
