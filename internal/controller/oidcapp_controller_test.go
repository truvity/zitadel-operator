package controller

// OIDCApp reconciler unit tests against an in-process fake Application
// service: the real zitadel-go SDK client is dialed over bufconn, so the
// exact protobuf requests the reconciler sends can be asserted without a
// live Zitadel.

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	zitadelclient "github.com/zitadel/zitadel-go/v3/pkg/client"
	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
	sdkzitadel "github.com/zitadel/zitadel-go/v3/pkg/zitadel"
)

// fakeApplicationServer records the requests the reconciler sends.
type fakeApplicationServer struct {
	applicationv2.UnimplementedApplicationServiceServer

	mu        sync.Mutex
	createReq *applicationv2.CreateApplicationRequest
	updateReq *applicationv2.UpdateApplicationRequest
}

func (f *fakeApplicationServer) CreateApplication(_ context.Context, req *applicationv2.CreateApplicationRequest) (*applicationv2.CreateApplicationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createReq = req
	return &applicationv2.CreateApplicationResponse{
		ApplicationId: "app-1",
		ApplicationType: &applicationv2.CreateApplicationResponse_OidcConfiguration{
			OidcConfiguration: &applicationv2.CreateOIDCApplicationResponse{
				ClientId:     "client-1",
				ClientSecret: "secret-1",
			},
		},
	}, nil
}

func (f *fakeApplicationServer) UpdateApplication(_ context.Context, req *applicationv2.UpdateApplicationRequest) (*applicationv2.UpdateApplicationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateReq = req
	return &applicationv2.UpdateApplicationResponse{}, nil
}

// newFakeZitadelClient serves fake over bufconn and returns an operator
// client whose SDK connection is dialed against it (static token, no OIDC
// discovery, no network).
func newFakeZitadelClient(t *testing.T, fake applicationv2.ApplicationServiceServer) *zitadel.Client {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	applicationv2.RegisterApplicationServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	inner, err := zitadelclient.New(context.Background(),
		sdkzitadel.New("bufconn", sdkzitadel.WithInsecure("0")),
		zitadelclient.WithAuth(func(_ context.Context, _ string) (oauth2.TokenSource, error) {
			return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}), nil
		}),
		zitadelclient.WithGRPCDialOptions(
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
		),
	)
	require.NoError(t, err)
	return zitadel.NewFromSDK(inner)
}

func testOIDCApp() *zitadelv1alpha2.OIDCApp {
	return &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "default"},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://app.example.com/callback"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: "my-app-oidc"},
		},
	}
}

func TestCreateOIDCApp_PropagatesIdTokenUserinfoAssertion(t *testing.T) {
	fake := &fakeApplicationServer{}
	r := &OIDCAppReconciler{Zitadel: newFakeZitadelClient(t, fake)}

	cr := testOIDCApp()
	cr.Spec.IdTokenRoleAssertion = true
	cr.Spec.IdTokenUserinfoAssertion = true

	appID, clientID, clientSecret, err := r.createOIDCApp(context.Background(), "proj-1", cr)
	require.NoError(t, err)
	assert.Equal(t, "app-1", appID)
	assert.Equal(t, "client-1", clientID)
	assert.Equal(t, "secret-1", clientSecret)

	require.NotNil(t, fake.createReq)
	oidcCfg := fake.createReq.GetOidcConfiguration()
	require.NotNil(t, oidcCfg)
	assert.True(t, oidcCfg.GetIdTokenRoleAssertion())
	assert.True(t, oidcCfg.GetIdTokenUserinfoAssertion(),
		"spec.idTokenUserinfoAssertion must reach the create request")
}

func TestUpdateOIDCAppIfNeeded_IdTokenUserinfoDriftTriggersUpdate(t *testing.T) {
	fake := &fakeApplicationServer{}
	r := &OIDCAppReconciler{Zitadel: newFakeZitadelClient(t, fake)}

	cr := testOIDCApp()
	cr.Spec.IdTokenUserinfoAssertion = true

	// Server state matches the spec except idTokenUserinfoAssertion.
	app := &applicationv2.Application{
		Name: cr.DisplayName(),
		Configuration: &applicationv2.Application_OidcConfiguration{
			OidcConfiguration: &applicationv2.OIDCConfiguration{
				RedirectUris:             cr.Spec.RedirectUris,
				AccessTokenType:          applicationv2.OIDCTokenType_OIDC_TOKEN_TYPE_BEARER,
				IdTokenUserinfoAssertion: false,
			},
		},
	}

	err := r.updateOIDCAppIfNeeded(context.Background(), "app-1", "proj-1", app, cr)
	require.NoError(t, err)

	require.NotNil(t, fake.updateReq, "drift on idTokenUserinfoAssertion must trigger an update")
	oidcCfg := fake.updateReq.GetOidcConfiguration()
	require.NotNil(t, oidcCfg)
	assert.True(t, oidcCfg.GetIdTokenUserinfoAssertion(),
		"update must carry the desired idTokenUserinfoAssertion value")
}

func TestUpdateOIDCAppIfNeeded_NoDriftNoUpdate(t *testing.T) {
	fake := &fakeApplicationServer{}
	r := &OIDCAppReconciler{Zitadel: newFakeZitadelClient(t, fake)}

	cr := testOIDCApp()
	cr.Spec.IdTokenUserinfoAssertion = true

	app := &applicationv2.Application{
		Name: cr.DisplayName(),
		Configuration: &applicationv2.Application_OidcConfiguration{
			OidcConfiguration: &applicationv2.OIDCConfiguration{
				RedirectUris:             cr.Spec.RedirectUris,
				AccessTokenType:          applicationv2.OIDCTokenType_OIDC_TOKEN_TYPE_BEARER,
				IdTokenUserinfoAssertion: true,
			},
		},
	}

	err := r.updateOIDCAppIfNeeded(context.Background(), "app-1", "proj-1", app, cr)
	require.NoError(t, err)
	assert.Nil(t, fake.updateReq, "matching state must not produce an update call")
}
