// Package zitadel provides a client wrapper around the official zitadel-go SDK
// for use by the operator controllers.
package zitadel

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/profile"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	zitadelclient "github.com/zitadel/zitadel-go/v3/pkg/client"
	actionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/action/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/auth"
	idpv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/idp/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	orgv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/org/v2"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
	settingsv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/settings/v2"
	userv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/zitadel"
	"golang.org/x/net/http2"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// Every Zitadel call the operator makes must be bounded: the SDK fetches an
// OAuth token via a per-RPC HTTP POST that ignores the RPC context (the token
// source's Token() runs on context.Background()), so an unbounded HTTP client
// on a silently-dropped connection wedges a controller worker forever — no
// error, no requeue, healthz still green.
const (
	// tokenHTTPTimeout bounds the whole OAuth token-endpoint request.
	tokenHTTPTimeout = 30 * time.Second
	// h2ReadIdleTimeout/h2PingTimeout health-check pooled HTTP/2 connections
	// to the token endpoint so a dead connection is closed and redialed
	// instead of queueing requests forever.
	h2ReadIdleTimeout = 30 * time.Second
	h2PingTimeout     = 15 * time.Second
	// defaultRPCTimeout is applied to any gRPC call whose context has no
	// deadline of its own.
	defaultRPCTimeout = 60 * time.Second
	// keepaliveTime/keepaliveTimeout detect dead gRPC transports.
	keepaliveTime    = 30 * time.Second
	keepaliveTimeout = 20 * time.Second
)

// Client wraps the official Zitadel SDK and exposes v2 service clients.
type Client struct {
	inner *zitadelclient.Client
}

// ClientConfig holds the configuration for creating a Zitadel client.
type ClientConfig struct {
	// Domain is the internal address of the Zitadel service (e.g., zitadel.zitadel.svc.cluster.kernel).
	Domain string

	// Port is the port for the Zitadel API.
	Port string

	// InsecurePlaintext connects without TLS (for in-cluster communication).
	InsecurePlaintext bool

	// KeyJSON is the JWT profile key data.
	KeyJSON []byte

	// ExternalDomain is the canonical external domain Zitadel is configured with
	// (e.g., zitadel.truvity.xyz). When set, enables split-horizon mode:
	// - Connects to Domain:Port (internal)
	// - Sends x-zitadel-instance-host header for instance matching
	// - Signs JWT assertions with audience https://ExternalDomain
	// - Uses static token endpoint (skips OIDC discovery)
	ExternalDomain string
}

// NewClient creates a new Zitadel client using JWT Profile authentication.
func NewClient(ctx context.Context, cfg *ClientConfig) (*Client, error) {
	keyFile, err := zitadelclient.ConfigFromKeyFileData(cfg.KeyJSON)
	if err != nil {
		return nil, fmt.Errorf("parsing key file: %w", err)
	}

	// Split-horizon mode: connect to internal service, authenticate against external domain.
	if cfg.ExternalDomain != "" {
		return newSplitHorizonClient(ctx, cfg, keyFile)
	}

	// Standard mode: direct connection.
	var zitadelOpts []zitadel.Option
	if cfg.InsecurePlaintext {
		zitadelOpts = append(zitadelOpts, zitadel.WithInsecure(cfg.Port))
	} else {
		zitadelOpts = append(zitadelOpts, zitadel.WithPort(mustParsePort(cfg.Port)))
	}

	httpClient, err := tokenHTTPClient(nil)
	if err != nil {
		return nil, fmt.Errorf("building token HTTP client: %w", err)
	}
	clientOpts := []zitadelclient.Option{
		// Not AuthenticationJWTProfile: the SDK builds a bare token source
		// that POSTs the token endpoint on every RPC with no HTTP timeout
		// (see the timeout constants above). Same JWT profile flow, but with
		// a bounded HTTP client and a ReuseTokenSource cache.
		zitadelclient.WithAuth(func(ctx context.Context, issuer string) (oauth2.TokenSource, error) {
			ts, err := profile.NewJWTProfileTokenSource(
				ctx, issuer, keyFile.UserID, keyFile.KeyID, keyFile.Key,
				[]string{oidc.ScopeOpenID, zitadelclient.ScopeZitadelAPI()},
				profile.WithHTTPClient(httpClient),
			)
			if err != nil {
				return nil, err
			}
			return oauth2.ReuseTokenSource(nil, ts), nil
		}),
		zitadelclient.WithGRPCDialOptions(boundedDialOptions()...),
	}
	if cfg.InsecurePlaintext {
		clientOpts = append(clientOpts,
			zitadelclient.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
	}

	inner, err := zitadelclient.New(ctx, zitadel.New(cfg.Domain, zitadelOpts...), clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating zitadel client: %w", err)
	}

	return &Client{inner: inner}, nil
}

// tokenHTTPClient returns the HTTP client used for OAuth token-endpoint
// requests: hard request timeout plus HTTP/2 ping health-checking, so a
// silently-dead pooled connection is detected and redialed instead of
// blocking token fetches (and with them every gRPC call) forever.
// A non-nil wrap decorates the health-checked transport (split-horizon
// header injection).
func tokenHTTPClient(wrap func(http.RoundTripper) http.RoundTripper) (*http.Client, error) {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("http.DefaultTransport is not *http.Transport")
	}
	transport = transport.Clone()
	h2, err := http2.ConfigureTransports(transport)
	if err != nil {
		return nil, fmt.Errorf("configuring HTTP/2 transport: %w", err)
	}
	h2.ReadIdleTimeout = h2ReadIdleTimeout
	h2.PingTimeout = h2PingTimeout

	inner := http.RoundTripper(transport)
	if wrap != nil {
		inner = wrap(transport)
	}
	return &http.Client{Timeout: tokenHTTPTimeout, Transport: inner}, nil
}

// boundedDialOptions returns the gRPC dial options every Zitadel connection
// gets: keepalive pings that fail dead transports, and a default deadline on
// any call whose context does not carry one.
func boundedDialOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                keepaliveTime,
			Timeout:             keepaliveTimeout,
			PermitWithoutStream: true,
		}),
		grpc.WithChainUnaryInterceptor(defaultTimeoutUnaryInterceptor(defaultRPCTimeout)),
	}
}

// defaultTimeoutUnaryInterceptor bounds deadline-less unary calls. Contexts
// that already carry a deadline (e.g. tests, callers with their own budget)
// pass through untouched.
func defaultTimeoutUnaryInterceptor(d time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// newSplitHorizonClient creates a client that connects to an internal service
// while authenticating against the external domain.
//
// Uses the standard OIDC library pattern:
// - WithStaticTokenEndpoint: skips discovery, points to internal token endpoint
// - x-zitadel-instance-host header: tells Zitadel which instance to use
// - JWT audience: https://ExternalDomain (what Zitadel expects)
func newSplitHorizonClient(ctx context.Context, cfg *ClientConfig, keyFile *zitadelclient.KeyFile) (*Client, error) {
	// Build the token source using the OIDC library directly.
	// This avoids the SDK's AuthenticationJWTProfile which ties issuer to the dial target.
	issuer := "https://" + cfg.ExternalDomain
	tokenEndpoint := "http://" + cfg.Domain + ":" + cfg.Port + "/oauth/v2/token"

	// HTTP client that adds the instance host header to token requests
	// (bounded + health-checked, see tokenHTTPClient).
	httpClient, err := tokenHTTPClient(func(inner http.RoundTripper) http.RoundTripper {
		return &instanceHeaderTransport{
			instanceHost: cfg.ExternalDomain,
			inner:        inner,
		}
	})
	if err != nil {
		return nil, fmt.Errorf("building token HTTP client: %w", err)
	}

	tokenSource, err := profile.NewJWTProfileTokenSource(
		ctx,
		issuer,
		keyFile.UserID,
		keyFile.KeyID,
		keyFile.Key,
		[]string{oidc.ScopeOpenID, zitadelclient.ScopeZitadelAPI()},
		profile.WithStaticTokenEndpoint(issuer, tokenEndpoint),
		profile.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("creating token source: %w", err)
	}
	// Cache tokens until expiry: without this the SDK POSTs the token
	// endpoint on every single RPC.
	cachedTokenSource := oauth2.ReuseTokenSource(nil, tokenSource)

	// SDK configuration: connect to internal domain, add instance header for gRPC.
	zitadelOpts := []zitadel.Option{
		zitadel.WithInsecure(cfg.Port),
		zitadel.WithTransportHeader("x-zitadel-instance-host", cfg.ExternalDomain),
	}

	clientOpts := []zitadelclient.Option{
		zitadelclient.WithAuth(func(_ context.Context, _ string) (oauth2.TokenSource, error) {
			return cachedTokenSource, nil
		}),
		zitadelclient.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
		zitadelclient.WithGRPCDialOptions(boundedDialOptions()...),
	}

	inner, err := zitadelclient.New(ctx, zitadel.New(cfg.Domain, zitadelOpts...), clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating zitadel client: %w", err)
	}

	return &Client{inner: inner}, nil
}

// instanceHeaderTransport adds the x-zitadel-instance-host header to HTTP requests.
// This tells Zitadel which instance should handle the request when connecting
// via an internal service domain.
type instanceHeaderTransport struct {
	instanceHost string
	inner        http.RoundTripper
}

func (t *instanceHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("x-zitadel-instance-host", t.instanceHost)
	return t.inner.RoundTrip(clone)
}

// Organization returns the v2 Organization service client.
func (c *Client) Organization() orgv2.OrganizationServiceClient {
	return c.inner.OrganizationServiceV2()
}

// Project returns the v2 Project service client.
func (c *Client) Project() projectv2.ProjectServiceClient {
	return c.inner.ProjectServiceV2()
}

// Application returns the v2 Application service client.
func (c *Client) Application() applicationv2.ApplicationServiceClient {
	return c.inner.ApplicationServiceV2()
}

// User returns the v2 User service client.
func (c *Client) User() userv2.UserServiceClient {
	return c.inner.UserServiceV2()
}

// IDP returns the v2 Identity Provider service client.
func (c *Client) IDP() idpv2.IdentityProviderServiceClient {
	return c.inner.IdpServiceV2()
}

// Settings returns the v2 Settings service client.
func (c *Client) Settings() settingsv2.SettingsServiceClient {
	return c.inner.SettingsServiceV2()
}

// Management returns the Management service client (for org-level operations).
func (c *Client) Management() management.ManagementServiceClient {
	return c.inner.ManagementService()
}

// Admin returns the Admin service client (for instance-level operations).
func (c *Client) Admin() admin.AdminServiceClient {
	return c.inner.AdminService()
}

// Auth returns the v1 Auth service client (self-service operations of the
// authenticated user, e.g. ListMyMemberships for startup binding checks).
func (c *Client) Auth() auth.AuthServiceClient {
	return c.inner.AuthService()
}

// Action returns the v2 Action service client.
func (c *Client) Action() actionv2.ActionServiceClient {
	return c.inner.ActionServiceV2()
}

func mustParsePort(port string) uint16 {
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil || p == 0 {
		return 443
	}
	return uint16(p)
}

// Ensure instanceHeaderTransport implements http.RoundTripper at compile time.
var _ http.RoundTripper = (*instanceHeaderTransport)(nil)

// Ensure the token source initializer signature matches what the SDK expects.
var _ zitadelclient.TokenSourceInitializer = func(_ context.Context, _ string) (oauth2.TokenSource, error) { return nil, nil }
