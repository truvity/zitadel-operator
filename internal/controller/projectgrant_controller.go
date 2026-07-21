package controller

import (
	"context"
	"fmt"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"
)

// ProjectGrantReconciler reconciles a ProjectGrant object.
type ProjectGrantReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectgrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectgrants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=projectgrants/finalizers,verbs=update

func (r *ProjectGrantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cr zitadelv1alpha2.ProjectGrant
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// v0.18 (INF-422/INF-423): dual-serving instance gate + scope
	// resolution. Fail-closed outcomes return immediately; during deletion
	// failures fall back to the binding client so finalizers cannot deadlock.
	ctx, rs, rsDone, rsResult, rsErr := tenantPreamble(ctx, r.Client, r.Config,
		r.Resolver, r.Delegation, r.Zitadel, &cr, cr.Spec.Instance, &cr.Status.Conditions, req.Namespace)
	if rsDone {
		return rsResult, rsErr
	}

	// Resolve organization (the owning org).
	orgID, err := resolveScopedOrganizationId(ctx, r.Client, rs, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "OrgNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Resolve project ID.
	projectID, _, err := resolveScopedProjectId(ctx, r.Client, rs, cr.Spec.ProjectRef, cr.Spec.ProjectId, cr.Namespace, "", "")
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "ProjectNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving project: %w", err)
	}

	// Resolve granted org ID.
	grantedOrgID, err := r.resolveGrantedOrgID(ctx, &cr)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "GrantedOrgNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving granted org: %w", err)
	}

	// Deletion.
	if done, result, err := handleDeletion(ctx, r.Client, &cr, func() error {
		return r.deleteGrant(ctx, projectID, cr.Status.GrantId)
	}); done {
		return result, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}
	// ensureFinalizer's full-object Update refreshed the object from the
	// server, dropping in-memory condition edits — re-apply ScopeResolved.
	applyScopeResolvedCondition(rs, &cr.Status.Conditions)

	// Ensure project grant exists.
	grantID, err := r.ensureProjectGrant(ctx, projectID, grantedOrgID, cr.Spec.RoleKeys, cr.Status.GrantId)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := cr.Status.GrantId != grantID
	cr.Status.GrantId = grantID
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ProjectGrantReconciler) deleteGrant(ctx context.Context, projectID, grantID string) error {
	if grantID == "" {
		return nil
	}
	_, err := zclient(ctx, r.Zitadel).Management().RemoveProjectGrant(ctx, &management.RemoveProjectGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		ProjectId: projectID,
		GrantId:   grantID,
	})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("removing project grant: %w", err)
	}
	return nil
}

func (r *ProjectGrantReconciler) resolveGrantedOrgID(ctx context.Context, cr *zitadelv1alpha2.ProjectGrant) (string, error) {
	if cr.Spec.GrantedOrgRef != nil && cr.Spec.GrantedOrgId != "" {
		return "", fmt.Errorf("grantedOrgRef and grantedOrgId are mutually exclusive")
	}
	if cr.Spec.GrantedOrgRef == nil && cr.Spec.GrantedOrgId == "" {
		return "", fmt.Errorf("one of grantedOrgRef or grantedOrgId is required")
	}
	if cr.Spec.GrantedOrgId != "" {
		return cr.Spec.GrantedOrgId, nil
	}

	ns := cr.Spec.GrantedOrgRef.Namespace
	if ns == "" {
		ns = cr.Namespace
	}
	var org zitadelv1alpha2.Organization
	if err := r.Get(ctx, client.ObjectKey{Name: cr.Spec.GrantedOrgRef.Name, Namespace: ns}, &org); err != nil {
		return "", fmt.Errorf("resolving grantedOrgRef %s/%s: %w", ns, cr.Spec.GrantedOrgRef.Name, err)
	}
	if org.Status.OrganizationId == "" {
		return "", fmt.Errorf("grantedOrgRef %s/%s not yet ready (no organizationId in status)", ns, cr.Spec.GrantedOrgRef.Name)
	}
	return org.Status.OrganizationId, nil
}

func (r *ProjectGrantReconciler) ensureProjectGrant(ctx context.Context, projectID, grantedOrgID string, desiredRoles []string, existingGrantID string) (string, error) {
	// If we have an existing grant ID, check it.
	if existingGrantID != "" {
		resp, err := zclient(ctx, r.Zitadel).Management().GetProjectGrantByID(ctx, &management.GetProjectGrantByIDRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
			ProjectId: projectID,
			GrantId:   existingGrantID,
		})
		if err == nil {
			// Grant exists, check if roles need updating.
			if !rolesEqual(resp.GetProjectGrant().GetGrantedRoleKeys(), desiredRoles) {
				_, err := zclient(ctx, r.Zitadel).Management().UpdateProjectGrant(ctx, &management.UpdateProjectGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
					ProjectId: projectID,
					GrantId:   existingGrantID,
					RoleKeys:  desiredRoles,
				})
				if err != nil {
					return "", fmt.Errorf("updating project grant: %w", err)
				}
			}
			return existingGrantID, nil
		}
		if status.Code(err) != codes.NotFound {
			return "", fmt.Errorf("getting project grant: %w", err)
		}
		// Grant was deleted externally, recreate.
	}

	// Create new grant.
	addResp, err := zclient(ctx, r.Zitadel).Management().AddProjectGrant(ctx, &management.AddProjectGrantRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		ProjectId:    projectID,
		GrantedOrgId: grantedOrgID,
		RoleKeys:     desiredRoles,
	})
	if err != nil {
		return "", fmt.Errorf("adding project grant: %w", err)
	}

	return addResp.GetGrantId(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectGrantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.ProjectGrant{}).
		Named("projectgrant").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
