package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
)

// OIDCAppReconciler reconciles an OIDCApp object.
type OIDCAppReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=oidcapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=oidcapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=oidcapps/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func (r *OIDCAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the OIDCApp CR.
	var cr zitadelv1alpha1.OIDCApp
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve project name → project ID.
	projectID, err := r.resolveProjectID(ctx, cr.Spec.Project)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolving project %q: %w", cr.Spec.Project, err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.ClientId != "" {
			appID, _ := r.findAppByName(ctx, projectID, cr.Name)
			if appID != "" {
				_, err := r.Zitadel.Application().DeleteApplication(ctx, &applicationv2.DeleteApplicationRequest{
					ApplicationId: appID,
					ProjectId:     projectID,
				})
				if err != nil && status.Code(err) != codes.NotFound {
					return ctrl.Result{}, fmt.Errorf("deleting application: %w", err)
				}
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

	// List apps in project, match by name.
	existingAppID, existingApp := r.findAppByName(ctx, projectID, cr.Name)

	var clientID, clientSecret string

	if existingAppID == "" {
		// Create OIDC app.
		clientID, clientSecret, _, err = r.createOIDCApp(ctx, projectID, &cr)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// Update redirect URIs if changed.
		clientID = r.getClientIDFromApp(existingApp)
		if err := r.updateOIDCAppIfNeeded(ctx, existingAppID, projectID, existingApp, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// For confidential apps: write client_id + client_secret to K8s Secret.
	if cr.Spec.Type == "confidential" && clientSecret != "" {
		if err := r.ensureSecret(ctx, &cr, clientID, clientSecret); err != nil {
			return ctrl.Result{}, err
		}
	} else if cr.Spec.Type == "confidential" && clientID != "" {
		// Ensure secret exists with at least client_id (secret may already be stored).
		if err := r.ensureSecretClientID(ctx, &cr, clientID); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.ClientId = clientID
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("oidcapp reconciled", "clientId", clientID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *OIDCAppReconciler) resolveProjectID(ctx context.Context, projectName string) (string, error) {
	listResp, err := r.Zitadel.Project().ListProjects(ctx, &projectv2.ListProjectsRequest{
		Filters: []*projectv2.ProjectSearchFilter{
			{
				Filter: &projectv2.ProjectSearchFilter_ProjectNameFilter{
					ProjectNameFilter: &projectv2.ProjectNameFilter{
						ProjectName: projectName,
						Method:      filterv2.TextFilterMethod_TEXT_FILTER_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}

	for _, p := range listResp.GetProjects() {
		if p.GetName() == projectName {
			return p.GetProjectId(), nil
		}
	}

	return "", fmt.Errorf("project %q not found", projectName)
}

func (r *OIDCAppReconciler) findAppByName(ctx context.Context, projectID, appName string) (string, *applicationv2.Application) {
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

func (r *OIDCAppReconciler) createOIDCApp(ctx context.Context, projectID string, cr *zitadelv1alpha1.OIDCApp) (clientID, clientSecret, appID string, err error) {
	appType := applicationv2.OIDCApplicationType_OIDC_APP_TYPE_WEB
	authMethod := applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_BASIC
	if cr.Spec.AuthMethod == "none" {
		authMethod = applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_NONE
	}
	if cr.Spec.Type == "public" {
		appType = applicationv2.OIDCApplicationType_OIDC_APP_TYPE_USER_AGENT
		authMethod = applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_NONE
	}

	// Resolve access token type.
	accessTokenType := applicationv2.OIDCTokenType_OIDC_TOKEN_TYPE_BEARER
	if cr.Spec.AccessTokenType == "jwt" {
		accessTokenType = applicationv2.OIDCTokenType_OIDC_TOKEN_TYPE_JWT
	}

	oidcConfig := &applicationv2.CreateOIDCApplicationRequest{
		RedirectUris:             cr.Spec.RedirectUris,
		PostLogoutRedirectUris:   cr.Spec.PostLogoutRedirectUris,
		ResponseTypes:            []applicationv2.OIDCResponseType{applicationv2.OIDCResponseType_OIDC_RESPONSE_TYPE_CODE},
		GrantTypes:               []applicationv2.OIDCGrantType{applicationv2.OIDCGrantType_OIDC_GRANT_TYPE_AUTHORIZATION_CODE},
		ApplicationType:          appType,
		AuthMethodType:           authMethod,
		AccessTokenType:          accessTokenType,
		AccessTokenRoleAssertion: cr.Spec.AccessTokenRoleAssertion,
		IdTokenRoleAssertion:     cr.Spec.IdTokenRoleAssertion,
	}

	resp, createErr := r.Zitadel.Application().CreateApplication(ctx, &applicationv2.CreateApplicationRequest{
		ProjectId: projectID,
		Name:      cr.Name,
		ApplicationType: &applicationv2.CreateApplicationRequest_OidcConfiguration{
			OidcConfiguration: oidcConfig,
		},
	})
	if createErr != nil {
		return "", "", "", fmt.Errorf("creating OIDC application: %w", createErr)
	}

	appID = resp.GetApplicationId()
	if oidcResp := resp.GetApplicationType(); oidcResp != nil {
		if oidcRespConfig, ok := oidcResp.(*applicationv2.CreateApplicationResponse_OidcConfiguration); ok {
			clientID = oidcRespConfig.OidcConfiguration.GetClientId()
			clientSecret = oidcRespConfig.OidcConfiguration.GetClientSecret()
		}
	}

	return clientID, clientSecret, appID, nil
}

func (r *OIDCAppReconciler) getClientIDFromApp(app *applicationv2.Application) string {
	if app == nil {
		return ""
	}
	if oidcConfig := app.GetOidcConfiguration(); oidcConfig != nil {
		return oidcConfig.GetClientId()
	}
	return ""
}

func (r *OIDCAppReconciler) updateOIDCAppIfNeeded(ctx context.Context, appID, projectID string, app *applicationv2.Application, cr *zitadelv1alpha1.OIDCApp) error {
	if app == nil {
		return nil
	}
	oidcConfig := app.GetOidcConfiguration()
	if oidcConfig == nil {
		return nil
	}

	// Check if redirect URIs changed.
	if reflect.DeepEqual(oidcConfig.GetRedirectUris(), cr.Spec.RedirectUris) {
		return nil
	}

	_, err := r.Zitadel.Application().UpdateApplication(ctx, &applicationv2.UpdateApplicationRequest{
		ApplicationId: appID,
		ProjectId:     projectID,
		Name:          cr.Name,
		ApplicationType: &applicationv2.UpdateApplicationRequest_OidcConfiguration{
			OidcConfiguration: &applicationv2.UpdateOIDCApplicationConfigurationRequest{
				RedirectUris: cr.Spec.RedirectUris,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("updating OIDC application: %w", err)
	}
	return nil
}

// clientIDKey returns the configurable key name for the client ID in the Secret.
// Defaults to "client_id" if not specified.
func clientIDKey(cr *zitadelv1alpha1.OIDCApp) string {
	if cr.Spec.SecretRef.Keys != nil && cr.Spec.SecretRef.Keys.ClientId != "" {
		return cr.Spec.SecretRef.Keys.ClientId
	}
	return "client_id"
}

// clientSecretKey returns the configurable key name for the client secret in the Secret.
// Defaults to "client_secret" if not specified.
func clientSecretKey(cr *zitadelv1alpha1.OIDCApp) string {
	if cr.Spec.SecretRef.Keys != nil && cr.Spec.SecretRef.Keys.ClientSecret != "" {
		return cr.Spec.SecretRef.Keys.ClientSecret
	}
	return "client_secret"
}

func (r *OIDCAppReconciler) ensureSecret(ctx context.Context, cr *zitadelv1alpha1.OIDCApp, clientID, clientSecret string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.SecretRef.Name,
			Namespace: cr.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		secret.Data[clientIDKey(cr)] = []byte(clientID)
		secret.Data[clientSecretKey(cr)] = []byte(clientSecret)
		// Write extra data entries into the Secret.
		for k, v := range cr.Spec.SecretRef.ExtraData {
			secret.Data[k] = []byte(v)
		}
		return nil
	})
	return err
}

func (r *OIDCAppReconciler) ensureSecretClientID(ctx context.Context, cr *zitadelv1alpha1.OIDCApp, clientID string) error {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      cr.Spec.SecretRef.Name,
		Namespace: cr.Namespace,
	}, secret)
	if err != nil {
		// Secret doesn't exist yet, create with client_id + extra data.
		if client.IgnoreNotFound(err) == nil {
			data := map[string][]byte{
				clientIDKey(cr): []byte(clientID),
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

	// Secret exists — ensure client_id and extra data are up to date.
	updated := false
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	idKey := clientIDKey(cr)
	if string(secret.Data[idKey]) != clientID {
		secret.Data[idKey] = []byte(clientID)
		updated = true
	}
	for k, v := range cr.Spec.SecretRef.ExtraData {
		if string(secret.Data[k]) != v {
			secret.Data[k] = []byte(v)
			updated = true
		}
	}
	if updated {
		return r.Update(ctx, secret)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OIDCAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.OIDCApp{}).
		Named("oidcapp").
		Complete(r)
}
