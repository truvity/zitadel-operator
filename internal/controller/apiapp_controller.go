package controller

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
)

// APIAppReconciler reconciles an APIApp object.
type APIAppReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=apiapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=apiapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=apiapps/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func (r *APIAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cr zitadelv1alpha2.APIApp
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Project scope label enforcement.
	if done, result, err := checkProjectScope(ctx, r.Client, r.Config, req.Namespace, &cr, &cr.Status.Conditions); done {
		return result, err
	}

	// Resolve project ID (and inherited org ID).
	projectID, inheritedOrgID, err := resolveProjectId(ctx, r.Client, cr.Spec.ProjectRef, cr.Spec.ProjectId, cr.Namespace)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "ProjectNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving project: %w", err)
	}

	// Handle deletion.
	if done, result, err := handleDeletionStrict(ctx, r.Client, &cr, func() error {
		if cr.Status.ApplicationId != "" {
			_, err := r.Zitadel.Application().DeleteApplication(ctx, &applicationv2.DeleteApplicationRequest{
				ApplicationId: cr.Status.ApplicationId,
				ProjectId:     projectID,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return fmt.Errorf("deleting application: %w", err)
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

	// Find or create app.
	displayName := cr.DisplayName()
	appID, clientID, clientSecret, err := r.findOrCreateApp(ctx, projectID, displayName, &cr)
	if err != nil {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "CreateFailed", err.Error())
		_ = applyStatus(ctx, r.Client, r.Config, &cr)
		return ctrl.Result{}, err
	}

	// Store credentials in Secret.
	if err := r.ensureCredentialSecret(ctx, &cr, clientID, clientSecret); err != nil {
		return ctrl.Result{}, err
	}

	// Update status only if changed.
	if err := r.updateStatusIfNeeded(ctx, &cr, appID, clientID, projectID, inheritedOrgID); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *APIAppReconciler) ensureCredentialSecret(ctx context.Context, cr *zitadelv1alpha2.APIApp, clientID, clientSecret string) error {
	if cr.Spec.AuthMethod == "basic" && clientSecret != "" {
		return r.ensureSecret(ctx, cr, clientID, clientSecret)
	}
	if clientID != "" {
		return r.ensureSecretClientID(ctx, cr, clientID)
	}
	return nil
}

func (r *APIAppReconciler) updateStatusIfNeeded(ctx context.Context, cr *zitadelv1alpha2.APIApp, appID, clientID, projectID, inheritedOrgID string) error {
	statusChanged := cr.Status.ApplicationId != appID || cr.Status.ClientId != clientID ||
		cr.Status.ProjectId != projectID || cr.Status.OrganizationId != inheritedOrgID
	cr.Status.ApplicationId = appID
	cr.Status.ClientId = clientID
	cr.Status.ProjectId = projectID
	cr.Status.OrganizationId = inheritedOrgID
	return markReady(ctx, r.Client, r.Config, cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged)
}

func (r *APIAppReconciler) findOrCreateApp(ctx context.Context, projectID, displayName string, cr *zitadelv1alpha2.APIApp) (appID, clientID, clientSecret string, err error) {
	existingAppID, existingApp := r.findAppByName(ctx, projectID, displayName)

	if existingAppID == "" {
		return r.createAPIApp(ctx, projectID, cr)
	}

	// Adoption path: the client secret of an existing app cannot be read back.
	// Regenerate it unless the referenced Secret already holds one.
	if cr.Spec.AuthMethod == "basic" {
		clientSecret, err = regenerateAdoptedClientSecret(ctx, r.Client, r.Zitadel.Application(),
			cr.Namespace, cr.Spec.SecretRef.Name, apiClientSecretKey(cr), projectID, existingAppID)
		if err != nil {
			return "", "", "", err
		}
	}
	return existingAppID, r.getClientIDFromApp(existingApp), clientSecret, nil
}

func (r *APIAppReconciler) findAppByName(ctx context.Context, projectID, appName string) (string, *applicationv2.Application) {
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
		return "", nil
	}

	for _, app := range listResp.GetApplications() {
		if app.GetName() == appName {
			appID := app.GetApplicationId()
			return appID, app
		}
	}
	return "", nil
}

func (r *APIAppReconciler) createAPIApp(ctx context.Context, projectID string, cr *zitadelv1alpha2.APIApp) (appID, clientID, clientSecret string, err error) {
	authMethod := applicationv2.APIAuthMethodType_API_AUTH_METHOD_TYPE_BASIC
	if cr.Spec.AuthMethod == "private_key_jwt" {
		authMethod = applicationv2.APIAuthMethodType_API_AUTH_METHOD_TYPE_PRIVATE_KEY_JWT
	}

	resp, createErr := r.Zitadel.Application().CreateApplication(ctx, &applicationv2.CreateApplicationRequest{
		ProjectId: projectID,
		Name:      cr.DisplayName(),
		ApplicationType: &applicationv2.CreateApplicationRequest_ApiConfiguration{
			ApiConfiguration: &applicationv2.CreateAPIApplicationRequest{
				AuthMethodType: authMethod,
			},
		},
	})
	if createErr != nil {
		return "", "", "", fmt.Errorf("creating API application: %w", createErr)
	}

	appID = resp.GetApplicationId()
	if apiResp := resp.GetApiConfiguration(); apiResp != nil {
		clientID = apiResp.GetClientId()
		clientSecret = apiResp.GetClientSecret()
	}

	return appID, clientID, clientSecret, nil
}

func (r *APIAppReconciler) getClientIDFromApp(app *applicationv2.Application) string {
	if app == nil {
		return ""
	}
	if apiConfig := app.GetApiConfiguration(); apiConfig != nil {
		return apiConfig.GetClientId()
	}
	return ""
}

func (r *APIAppReconciler) ensureSecret(ctx context.Context, cr *zitadelv1alpha2.APIApp, clientID, clientSecret string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.SecretRef.Name,
			Namespace: cr.Namespace,
		},
	}

	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: cr.Spec.SecretRef.Name, Namespace: cr.Namespace}, existing)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			secret.Data = map[string][]byte{
				apiClientIDKey(cr):     []byte(clientID),
				apiClientSecretKey(cr): []byte(clientSecret),
			}
			for k, v := range cr.Spec.SecretRef.ExtraData {
				secret.Data[k] = []byte(v)
			}
			return r.Create(ctx, secret)
		}
		return err
	}

	if existing.Data == nil {
		existing.Data = make(map[string][]byte)
	}
	existing.Data[apiClientIDKey(cr)] = []byte(clientID)
	existing.Data[apiClientSecretKey(cr)] = []byte(clientSecret)
	for k, v := range cr.Spec.SecretRef.ExtraData {
		existing.Data[k] = []byte(v)
	}
	return r.Update(ctx, existing)
}

func (r *APIAppReconciler) ensureSecretClientID(ctx context.Context, cr *zitadelv1alpha2.APIApp, clientID string) error {
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: cr.Spec.SecretRef.Name, Namespace: cr.Namespace}, existing)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			data := map[string][]byte{
				apiClientIDKey(cr): []byte(clientID),
			}
			for k, v := range cr.Spec.SecretRef.ExtraData {
				data[k] = []byte(v)
			}
			newSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cr.Spec.SecretRef.Name,
					Namespace: cr.Namespace,
				},
				Data: data,
			}
			return r.Create(ctx, newSecret)
		}
		return err
	}

	updated := false
	if existing.Data == nil {
		existing.Data = make(map[string][]byte)
	}
	idKey := apiClientIDKey(cr)
	if string(existing.Data[idKey]) != clientID {
		existing.Data[idKey] = []byte(clientID)
		updated = true
	}
	for k, v := range cr.Spec.SecretRef.ExtraData {
		if string(existing.Data[k]) != v {
			existing.Data[k] = []byte(v)
			updated = true
		}
	}
	if updated {
		return r.Update(ctx, existing)
	}
	return nil
}

func apiClientIDKey(cr *zitadelv1alpha2.APIApp) string {
	if cr.Spec.SecretRef.Keys != nil && cr.Spec.SecretRef.Keys.ClientId != "" {
		return cr.Spec.SecretRef.Keys.ClientId
	}
	return "client_id"
}

func apiClientSecretKey(cr *zitadelv1alpha2.APIApp) string {
	if cr.Spec.SecretRef.Keys != nil && cr.Spec.SecretRef.Keys.ClientSecret != "" {
		return cr.Spec.SecretRef.Keys.ClientSecret
	}
	return "client_secret"
}

// SetupWithManager sets up the controller with the Manager.
func (r *APIAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.APIApp{}).
		Named("apiapp").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
