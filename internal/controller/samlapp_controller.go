package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
)

// SAMLAppReconciler reconciles a SAMLApp object.
type SAMLAppReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=samlapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=samlapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=samlapps/finalizers,verbs=update

func (r *SAMLAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.SAMLApp
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Project scope label enforcement.
	shouldProceed, err := validateProjectScope(ctx, r.Client, r.Config, req.Namespace, &cr.Status.Conditions)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !shouldProceed {
		if statusErr := r.Status().Update(ctx, &cr); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		logger.Info("project scope validation failed, requeueing",
			"namespace", req.Namespace,
			"label", r.Config.ProjectScopeLabel)
		return ctrl.Result{RequeueAfter: requeueOnError}, nil
	}

	// Resolve project ID (and inherited org ID).
	projectID, inheritedOrgID, err := resolveProjectId(ctx, r.Client, cr.Spec.ProjectRef, cr.Spec.ProjectId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for project ref to become ready", "error", err)
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "ProjectNotReady", err.Error())
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving project: %w", err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.ApplicationId != "" {
			_, err := r.Zitadel.Application().DeleteApplication(ctx, &applicationv2.DeleteApplicationRequest{
				ApplicationId: cr.Status.ApplicationId,
				ProjectId:     projectID,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("deleting SAML application: %w", err)
			}
		}
		if removeFinalizer(&cr) {
			if err := r.Update(ctx, &cr); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present.
	if addFinalizer(&cr) {
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Find or create app.
	displayName := cr.DisplayName()
	existingAppID := r.findAppByName(ctx, projectID, displayName)

	var appID string

	if existingAppID == "" {
		appID, err = r.createSAMLApp(ctx, projectID, &cr)
		if err != nil {
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "CreateFailed", err.Error())
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{}, err
		}
	} else {
		appID = existingAppID
	}

	// Update status only if changed.
	statusChanged := cr.Status.ApplicationId != appID ||
		cr.Status.ProjectId != projectID || cr.Status.OrganizationId != inheritedOrgID || !cr.Status.Ready
	if statusChanged {
		now := metav1.NewTime(time.Now())
		cr.Status.ApplicationId = appID
		cr.Status.ProjectId = projectID
		cr.Status.OrganizationId = inheritedOrgID
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("samlapp reconciled", "appId", appID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *SAMLAppReconciler) findAppByName(ctx context.Context, projectID, appName string) string {
	listResp, err := r.Zitadel.Application().ListApplications(ctx, &applicationv2.ListApplicationsRequest{
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

	resp, err := r.Zitadel.Application().CreateApplication(ctx, &applicationv2.CreateApplicationRequest{
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
