package controller

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
)

// SAMLAppReconciler reconciles a SAMLApp object.
type SAMLAppReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=samlapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=samlapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=samlapps/finalizers,verbs=update

func (r *SAMLAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cr zitadelv1alpha2.SAMLApp
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

	// Resolve project ID (and inherited org ID).
	projectID, inheritedOrgID, err := resolveScopedProjectId(ctx, r.Client, rs, cr.Spec.ProjectRef, cr.Spec.ProjectId, cr.Namespace, cr.Status.ProjectId, cr.Status.OrganizationId)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "ProjectNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving project: %w", err)
	}

	// Handle deletion.
	if done, result, err := handleDeletionStrict(ctx, r.Client, &cr, func() error {
		if cr.Status.ApplicationId != "" {
			_, err := zclient(ctx, r.Zitadel).Application().DeleteApplication(ctx, &applicationv2.DeleteApplicationRequest{
				ApplicationId: cr.Status.ApplicationId,
				ProjectId:     projectID,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return fmt.Errorf("deleting SAML application: %w", err)
			}
		}
		return nil
	}); done {
		return result, err
	}

	// Add finalizer if not present.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}
	// ensureFinalizer's full-object Update refreshed the object from the
	// server, dropping in-memory condition edits — re-apply ScopeResolved.
	applyScopeResolvedCondition(rs, &cr.Status.Conditions)

	// Find or create app.
	displayName := cr.DisplayName()
	existingAppID := r.findAppByName(ctx, projectID, displayName)

	var appID string

	if existingAppID == "" {
		appID, err = r.createSAMLApp(ctx, projectID, &cr)
		if err != nil {
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "CreateFailed", err.Error())
			_ = applyStatus(ctx, r.Client, r.Config, &cr)
			return ctrl.Result{}, err
		}
	} else {
		appID = existingAppID
	}

	// Update status only if changed.
	statusChanged := cr.Status.ApplicationId != appID ||
		cr.Status.ProjectId != projectID || cr.Status.OrganizationId != inheritedOrgID
	cr.Status.ApplicationId = appID
	cr.Status.ProjectId = projectID
	cr.Status.OrganizationId = inheritedOrgID
	return ctrl.Result{RequeueAfter: requeueInterval}, markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged)
}

func (r *SAMLAppReconciler) findAppByName(ctx context.Context, projectID, appName string) string {
	listResp, err := zclient(ctx, r.Zitadel).Application().ListApplications(ctx, &applicationv2.ListApplicationsRequest{
		Filters: []*applicationv2.ApplicationSearchFilter{
			{
				Filter: &applicationv2.ApplicationSearchFilter_ProjectIdFilter{
					ProjectIdFilter: &applicationv2.ProjectIDFilter{
						ProjectId: projectID,
					},
				},
			},
		},
	})
	if err != nil {
		return ""
	}

	for _, app := range listResp.GetApplications() {
		if app.GetName() == appName {
			return app.GetApplicationId()
		}
	}
	return ""
}

func (r *SAMLAppReconciler) createSAMLApp(ctx context.Context, projectID string, cr *zitadelv1alpha2.SAMLApp) (string, error) {
	samlReq := &applicationv2.CreateSAMLApplicationRequest{}

	switch {
	case cr.Spec.MetadataXml != "":
		samlReq.Metadata = &applicationv2.CreateSAMLApplicationRequest_MetadataXml{
			MetadataXml: []byte(cr.Spec.MetadataXml),
		}
	case cr.Spec.MetadataUrl != "":
		samlReq.Metadata = &applicationv2.CreateSAMLApplicationRequest_MetadataUrl{
			MetadataUrl: cr.Spec.MetadataUrl,
		}
	default:
		return "", fmt.Errorf("one of metadataXml or metadataUrl is required")
	}

	resp, err := zclient(ctx, r.Zitadel).Application().CreateApplication(ctx, &applicationv2.CreateApplicationRequest{
		ProjectId: projectID,
		Name:      cr.DisplayName(),
		ApplicationType: &applicationv2.CreateApplicationRequest_SamlConfiguration{
			SamlConfiguration: samlReq,
		},
	})
	if err != nil {
		return "", fmt.Errorf("creating SAML application: %w", err)
	}

	return resp.GetApplicationId(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SAMLAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.SAMLApp{}).
		Named("samlapp").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
