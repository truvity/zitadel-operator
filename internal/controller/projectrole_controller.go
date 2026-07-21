package controller

import (
	"context"
	"fmt"

	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
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

// ProjectRoleReconciler reconciles a ProjectRole object (v0.19): one role
// key per CR, reconciled into a project named by projectRef/projectId (or
// the namespace's resolved scope project). Unlike Project spec.roles — an
// authoritative full-set sync — ProjectRole manages exactly its own key, so
// several namespaces (or charts) can contribute roles to one project without
// coordinating a single list. Downstream this carries the mechanical
// "{namespace}:{role}" role vocabulary.
type ProjectRoleReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectroles/finalizers,verbs=update

func (r *ProjectRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.ProjectRole
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// v0.18/v0.19 entry gates + scope resolution.
	ctx, rs, rsDone, rsResult, rsErr := tenantPreamble(ctx, r.Client, r.Config,
		r.Resolver, r.Delegation, r.Zitadel, &cr, cr.Spec.Instance, &cr.Status.Conditions, req.Namespace)
	if rsDone {
		return rsResult, rsErr
	}

	// Deletion — resolved from status so cleanup works even after the
	// referenced Project CR (and possibly the project itself) is gone.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		if cr.Status.ProjectId == "" || cr.Status.Key == "" {
			return nil
		}
		return r.removeRole(ctx, cr.Status.ProjectId, cr.Status.Key)
	}); done {
		return result, err
	}

	// Resolve the project: explicit ref/ID, the scope's project, or the
	// previously recorded status.projectId.
	projectID, resDone, resResult, err := r.resolveProject(ctx, &cr, rs)
	if resDone || err != nil {
		return resResult, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}
	// ensureFinalizer's full-object Update refreshed the object from the
	// server, dropping in-memory condition edits — re-apply ScopeResolved.
	applyScopeResolvedCondition(rs, &cr.Status.Conditions)

	// A changed role key (or project) removes the previously managed role
	// (the key is immutable in Zitadel — replace, don't rename).
	key := cr.RoleKey()
	if err := r.removeReplacedRole(ctx, &cr, projectID, key); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure the role exists with the desired display name and group
	// (create-or-adopt: an existing role with the same key is adopted and
	// drift-corrected).
	if err := r.ensureRole(ctx, &cr, projectID, key); err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := projectRoleStatusChanged(&cr.Status, projectID, key, cr.Generation)
	cr.Status.ProjectId = projectID
	cr.Status.Key = key
	cr.Status.ObservedGeneration = cr.Generation
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("projectrole reconciled", "projectId", projectID, "key", key)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// resolveProject resolves the target project and translates resolution
// failures into conditions: ProjectNotReady (transient — the referenced
// Project CR has no projectId yet) requeues politely; mutual exclusion /
// neither-set are steady-state spec errors and fail closed as InvalidSpec.
// done=true means the caller must return (result, err) immediately.
func (r *ProjectRoleReconciler) resolveProject(ctx context.Context, cr *zitadelv1alpha2.ProjectRole, rs resolvedScope) (projectID string, done bool, result ctrl.Result, err error) {
	logger := log.FromContext(ctx)
	projectID, _, resErr := resolveScopedProjectId(ctx, r.Client, rs, cr.Spec.ProjectRef, cr.Spec.ProjectId, cr.Namespace, cr.Status.ProjectId, "")
	if resErr == nil {
		return projectID, false, ctrl.Result{}, nil
	}
	if isRefNotReady(resErr) || isRefNotFound(resErr) {
		logger.Info("waiting for project ref to become ready", "error", resErr)
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "ProjectNotReady", resErr.Error())
		cr.Status.Ready = false
		_ = applyStatus(ctx, r.Client, r.Config, cr)
		return "", true, ctrl.Result{RequeueAfter: requeueOnError}, nil
	}
	setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "InvalidSpec", resErr.Error())
	cr.Status.Ready = false
	if applyErr := applyStatus(ctx, r.Client, r.Config, cr); applyErr != nil {
		return "", true, ctrl.Result{}, applyErr
	}
	logger.Info("project resolution rejected, fail-closed (InvalidSpec)", "error", resErr)
	return "", true, ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// removeReplacedRole drops the previously managed role when the effective
// key or target project changed since the last sync.
func (r *ProjectRoleReconciler) removeReplacedRole(ctx context.Context, cr *zitadelv1alpha2.ProjectRole, projectID, key string) error {
	if cr.Status.Key == "" || cr.Status.ProjectId == "" {
		return nil
	}
	if cr.Status.Key == key && cr.Status.ProjectId == projectID {
		return nil
	}
	if err := r.removeRole(ctx, cr.Status.ProjectId, cr.Status.Key); err != nil {
		return fmt.Errorf("removing replaced role %q: %w", cr.Status.Key, err)
	}
	return nil
}

// projectRoleStatusChanged reports whether a persisted status field moved.
func projectRoleStatusChanged(s *zitadelv1alpha2.ProjectRoleStatus, projectID, key string, generation int64) bool {
	return s.ProjectId != projectID || s.Key != key || s.ObservedGeneration != generation
}

// ensureRole creates the role if missing, or drift-corrects displayName and
// group on an existing (adopted) role.
func (r *ProjectRoleReconciler) ensureRole(ctx context.Context, cr *zitadelv1alpha2.ProjectRole, projectID, key string) error {
	displayName := cr.RoleDisplayName()

	listResp, err := zclient(ctx, r.Zitadel).Project().ListProjectRoles(ctx, &projectv2.ListProjectRolesRequest{
		ProjectId: projectID,
		Filters: []*projectv2.ProjectRoleSearchFilter{
			{
				Filter: &projectv2.ProjectRoleSearchFilter_RoleKeyFilter{
					RoleKeyFilter: &projectv2.ProjectRoleKeyFilter{
						Key:    key,
						Method: filterv2.TextFilterMethod_TEXT_FILTER_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("listing project roles: %w", err)
	}

	for _, role := range listResp.GetProjectRoles() {
		if role.GetKey() != key {
			continue
		}
		// Adopt; correct drift.
		if role.GetDisplayName() == displayName && role.GetGroup() == cr.Spec.Group {
			return nil
		}
		group := cr.Spec.Group
		_, err := zclient(ctx, r.Zitadel).Project().UpdateProjectRole(ctx, &projectv2.UpdateProjectRoleRequest{
			ProjectId:   projectID,
			RoleKey:     key,
			DisplayName: &displayName,
			Group:       &group,
		})
		if err != nil {
			return fmt.Errorf("updating project role %q: %w", key, err)
		}
		return nil
	}

	req := &projectv2.AddProjectRoleRequest{
		ProjectId:   projectID,
		RoleKey:     key,
		DisplayName: displayName,
	}
	if cr.Spec.Group != "" {
		req.Group = &cr.Spec.Group
	}
	if _, err := zclient(ctx, r.Zitadel).Project().AddProjectRole(ctx, req); err != nil {
		return fmt.Errorf("adding project role %q: %w", key, err)
	}
	return nil
}

// removeRole deletes the role, tolerating NotFound.
func (r *ProjectRoleReconciler) removeRole(ctx context.Context, projectID, key string) error {
	_, err := zclient(ctx, r.Zitadel).Project().RemoveProjectRole(ctx, &projectv2.RemoveProjectRoleRequest{
		ProjectId: projectID,
		RoleKey:   key,
	})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("removing project role %q: %w", key, err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.ProjectRole{}).
		Named("projectrole").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
