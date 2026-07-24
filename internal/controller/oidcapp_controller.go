package controller

import (
	"context"
	"fmt"
	"reflect"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
)

// OIDCAppReconciler reconciles an OIDCApp object.
type OIDCAppReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config

	// Resolver enables v0.18 scope-map resolution when non-nil; with maps
	// present, reconciliation runs with a delegated per-scope client.
	Resolver *scopemap.Resolver
	// Delegation mints/caches the per-scope delegated clients.
	Delegation *delegation.Manager
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=oidcapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=oidcapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=oidcapps/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func (r *OIDCAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cr zitadelv1alpha2.OIDCApp
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
	// ensureFinalizer's full-object Update refreshed the object from the
	// server, dropping in-memory condition edits — re-apply ScopeResolved.
	applyScopeResolvedCondition(rs, &cr.Status.Conditions)

	// Find or create app.
	displayName := cr.DisplayName()
	appID, clientID, clientSecret, err := r.findOrCreateApp(ctx, projectID, displayName, &cr)
	if err != nil {
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

func (r *OIDCAppReconciler) ensureCredentialSecret(ctx context.Context, cr *zitadelv1alpha2.OIDCApp, clientID, clientSecret string) error {
	if cr.Spec.Type == "confidential" && clientSecret != "" {
		return r.ensureSecret(ctx, cr, clientID, clientSecret)
	}
	if cr.Spec.Type == "confidential" && clientID != "" {
		return r.ensureSecretClientID(ctx, cr, clientID)
	}
	return nil
}

func (r *OIDCAppReconciler) updateStatusIfNeeded(ctx context.Context, cr *zitadelv1alpha2.OIDCApp, appID, clientID, projectID, inheritedOrgID string) error {
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

func (r *OIDCAppReconciler) findOrCreateApp(ctx context.Context, projectID, displayName string, cr *zitadelv1alpha2.OIDCApp) (appID, clientID, clientSecret string, err error) {
	existingAppID, existingApp := r.findAppByName(ctx, projectID, displayName)

	if existingAppID == "" {
		return r.createOIDCApp(ctx, projectID, cr)
	}

	appID = existingAppID
	clientID = r.getClientIDFromApp(existingApp)
	if err := r.updateOIDCAppIfNeeded(ctx, existingAppID, projectID, existingApp, cr); err != nil {
		return "", "", "", err
	}
	// Adoption path: the client secret of an existing app cannot be read back.
	// Regenerate it unless the referenced Secret already holds one.
	if cr.Spec.Type == "confidential" && cr.Spec.AuthMethod != "none" {
		clientSecret, err = regenerateAdoptedClientSecret(ctx, r.Client, zclient(ctx, r.Zitadel).Application(),
			cr.Namespace, cr.Spec.SecretRef.Name, clientSecretKey(cr), projectID, existingAppID)
		if err != nil {
			return "", "", "", err
		}
	}
	return appID, clientID, clientSecret, nil
}

func (r *OIDCAppReconciler) findAppByName(ctx context.Context, projectID, appName string) (string, *applicationv2.Application) {
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

func (r *OIDCAppReconciler) createOIDCApp(ctx context.Context, projectID string, cr *zitadelv1alpha2.OIDCApp) (appID, clientID, clientSecret string, err error) {
	appType := applicationv2.OIDCApplicationType_OIDC_APP_TYPE_WEB
	authMethod := applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_BASIC
	if cr.Spec.AuthMethod == "none" {
		authMethod = applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_NONE
	}
	if cr.Spec.Type == "public" {
		appType = applicationv2.OIDCApplicationType_OIDC_APP_TYPE_USER_AGENT
		authMethod = applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_NONE
	}

	accessTokenType := applicationv2.OIDCTokenType_OIDC_TOKEN_TYPE_BEARER
	if cr.Spec.AccessTokenType == "jwt" {
		accessTokenType = applicationv2.OIDCTokenType_OIDC_TOKEN_TYPE_JWT
	}

	resp, createErr := zclient(ctx, r.Zitadel).Application().CreateApplication(ctx, &applicationv2.CreateApplicationRequest{
		ProjectId: projectID,
		Name:      cr.DisplayName(),
		ApplicationType: &applicationv2.CreateApplicationRequest_OidcConfiguration{
			OidcConfiguration: &applicationv2.CreateOIDCApplicationRequest{
				RedirectUris:             cr.Spec.RedirectUris,
				PostLogoutRedirectUris:   cr.Spec.PostLogoutRedirectUris,
				ResponseTypes:            []applicationv2.OIDCResponseType{applicationv2.OIDCResponseType_OIDC_RESPONSE_TYPE_CODE},
				GrantTypes:               []applicationv2.OIDCGrantType{applicationv2.OIDCGrantType_OIDC_GRANT_TYPE_AUTHORIZATION_CODE},
				ApplicationType:          appType,
				AuthMethodType:           authMethod,
				AccessTokenType:          accessTokenType,
				AccessTokenRoleAssertion: cr.Spec.AccessTokenRoleAssertion,
				IdTokenRoleAssertion:     cr.Spec.IdTokenRoleAssertion,
				IdTokenUserinfoAssertion: cr.Spec.IdTokenUserinfoAssertion,
			},
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

	return appID, clientID, clientSecret, nil
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

func (r *OIDCAppReconciler) updateOIDCAppIfNeeded(ctx context.Context, appID, projectID string, app *applicationv2.Application, cr *zitadelv1alpha2.OIDCApp) error {
	if app == nil {
		return nil
	}
	oidcConfig := app.GetOidcConfiguration()
	if oidcConfig == nil {
		return nil
	}

	// Detect drift across all mutable fields.
	redirectsChanged := !reflect.DeepEqual(oidcConfig.GetRedirectUris(), cr.Spec.RedirectUris)
	postLogoutChanged := !reflect.DeepEqual(oidcConfig.GetPostLogoutRedirectUris(), cr.Spec.PostLogoutRedirectUris)

	desiredAccessTokenType := applicationv2.OIDCTokenType_OIDC_TOKEN_TYPE_BEARER
	if cr.Spec.AccessTokenType == "jwt" {
		desiredAccessTokenType = applicationv2.OIDCTokenType_OIDC_TOKEN_TYPE_JWT
	}
	accessTokenTypeChanged := oidcConfig.GetAccessTokenType() != desiredAccessTokenType

	accessTokenRoleChanged := oidcConfig.GetAccessTokenRoleAssertion() != cr.Spec.AccessTokenRoleAssertion
	idTokenRoleChanged := oidcConfig.GetIdTokenRoleAssertion() != cr.Spec.IdTokenRoleAssertion
	idTokenUserinfoChanged := oidcConfig.GetIdTokenUserinfoAssertion() != cr.Spec.IdTokenUserinfoAssertion

	if !redirectsChanged && !postLogoutChanged && !accessTokenTypeChanged && !accessTokenRoleChanged && !idTokenRoleChanged && !idTokenUserinfoChanged {
		return nil
	}

	logger := log.FromContext(ctx)
	logger.Info("drift detected, updating OIDC app",
		"redirectsChanged", redirectsChanged,
		"postLogoutChanged", postLogoutChanged,
		"accessTokenTypeChanged", accessTokenTypeChanged,
		"accessTokenRoleChanged", accessTokenRoleChanged,
		"idTokenRoleChanged", idTokenRoleChanged,
		"idTokenUserinfoChanged", idTokenUserinfoChanged,
	)

	// INF-400 root cause: sending the (unchanged) Name alongside a changed
	// OIDC configuration makes Zitadel's name-change command reject the whole
	// request with "No changes (COMMAND-2m8vx)" — the URI list then never
	// converges. Per the API contract, an unset Name leaves the name alone,
	// so it is only included when it actually drifted.
	updateName := ""
	if app.GetName() != cr.DisplayName() {
		updateName = cr.DisplayName()
	}
	_, err := zclient(ctx, r.Zitadel).Application().UpdateApplication(ctx, &applicationv2.UpdateApplicationRequest{
		ApplicationId: appID,
		ProjectId:     projectID,
		Name:          updateName,
		ApplicationType: &applicationv2.UpdateApplicationRequest_OidcConfiguration{
			OidcConfiguration: &applicationv2.UpdateOIDCApplicationConfigurationRequest{
				RedirectUris:             cr.Spec.RedirectUris,
				PostLogoutRedirectUris:   cr.Spec.PostLogoutRedirectUris,
				AccessTokenType:          &desiredAccessTokenType,
				AccessTokenRoleAssertion: &cr.Spec.AccessTokenRoleAssertion,
				IdTokenRoleAssertion:     &cr.Spec.IdTokenRoleAssertion,
				IdTokenUserinfoAssertion: &cr.Spec.IdTokenUserinfoAssertion,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("updating OIDC application: %w", err)
	}
	return nil
}

func clientIDKey(cr *zitadelv1alpha2.OIDCApp) string {
	if cr.Spec.SecretRef.Keys != nil && cr.Spec.SecretRef.Keys.ClientId != "" {
		return cr.Spec.SecretRef.Keys.ClientId
	}
	return "client_id"
}

func clientSecretKey(cr *zitadelv1alpha2.OIDCApp) string {
	if cr.Spec.SecretRef.Keys != nil && cr.Spec.SecretRef.Keys.ClientSecret != "" {
		return cr.Spec.SecretRef.Keys.ClientSecret
	}
	return "client_secret"
}

func (r *OIDCAppReconciler) ensureSecret(ctx context.Context, cr *zitadelv1alpha2.OIDCApp, clientID, clientSecret string) error {
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
		for k, v := range cr.Spec.SecretRef.ExtraData {
			secret.Data[k] = []byte(v)
		}
		return nil
	})
	return err
}

func (r *OIDCAppReconciler) ensureSecretClientID(ctx context.Context, cr *zitadelv1alpha2.OIDCApp, clientID string) error {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      cr.Spec.SecretRef.Name,
		Namespace: cr.Namespace,
	}, secret)
	if err != nil {
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
		For(&zitadelv1alpha2.OIDCApp{}).
		Named("oidcapp").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
