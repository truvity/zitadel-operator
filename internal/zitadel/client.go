// Package zitadel provides a client wrapper around the official zitadel-go SDK
// for use by the operator controllers.
package zitadel

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/client"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	zitadelclient "github.com/zitadel/zitadel-go/v3/pkg/client"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
	idpv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/idp/v2"
	orgv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/org/v2"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
	settingsv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/settings/v2"
	userv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/zitadel"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the official Zitadel SDK and exposes v2 service clients.
type Client struct {
	inner *zitadelclient.Client
}

// ClientConfig holds the configuration for creating a Zitadel client.
type ClientConfig struct {
	// Domain is the domain Zitadel recognizes as its instance (external domain).
	// Used for OIDC discovery and gRPC :authority header.
	Domain string

	// Port is the port for the Zitadel API.
	Port string

	// InsecurePlaintext connects without TLS (for in-cluster communication).
	InsecurePlaintext bool

	// KeyJSON is the JWT profile key data.
	KeyJSON []byte

	// TargetAddr is the actual network address to connect to (e.g., internal K8s service).
	// When set, DNS is hijacked so Domain resolves to TargetAddr, and a custom
	// token source signs JWTs with the correct HTTPS audience.
	// When empty, the SDK dials Domain directly.
	TargetAddr string
}

// NewClient creates a new Zitadel client using JWT Profile authentication.
// If cfg.TargetAddr is set, uses DNS hijacking + custom token source for
// split-horizon routing (internal network, external identity).
func NewClient(ctx context.Context, cfg *ClientConfig) (*Client, error) {
	keyFile, err := zitadelclient.ConfigFromKeyFileData(cfg.KeyJSON)
	if err != nil {
		return nil, fmt.Errorf("parsing key file: %w", err)
	}

	// When TargetAddr is set, use split-horizon mode:
	// - DNS hijack: Domain resolves to TargetAddr
	// - WithInsecure: plaintext gRPC transport
	// - Custom token source: signs JWT with https://Domain audience
	if cfg.TargetAddr != "" {
		return newSplitHorizonClient(ctx, cfg, keyFile)
	}

	// Standard mode: direct connection.
	var zitadelOpts []zitadel.Option
	if cfg.InsecurePlaintext {
		zitadelOpts = append(zitadelOpts, zitadel.WithInsecure(cfg.Port))
	} else {
		zitadelOpts = append(zitadelOpts, zitadel.WithPort(mustParsePort(cfg.Port)))
	}

	clientOpts := []zitadelclient.Option{
		zitadelclient.WithAuth(zitadelclient.AuthenticationJWTProfile(
			keyFile,
			oidc.ScopeOpenID,
			zitadelclient.ScopeZitadelAPI(),
		)),
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

// newSplitHorizonClient creates a client that connects to an internal service
// while presenting the external domain identity to Zitadel.
func newSplitHorizonClient(ctx context.Context, cfg *ClientConfig, _ *zitadelclient.KeyFile) (*Client, error) {
	// 1. DNS hijack: resolve cfg.Domain to cfg.TargetAddr.
	targetHost, targetPort, err := net.SplitHostPort(cfg.TargetAddr)
	if err != nil {
		// TargetAddr might not have a port — treat as host:cfg.Port
		targetHost = cfg.TargetAddr
		targetPort = cfg.Port
	}
	installDNSOverride(cfg.Domain, targetHost, targetPort)

	// 2. SDK in insecure mode — uses cfg.Domain:cfg.Port as dial target.
	// DNS hijack routes it to the internal service.
	zitadelOpts := []zitadel.Option{
		zitadel.WithInsecure(cfg.Port),
	}

	// 3. Custom token source: signs JWT with https://Domain audience.
	tokenSource, err := newSplitHorizonTokenSource(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating token source: %w", err)
	}

	clientOpts := []zitadelclient.Option{
		zitadelclient.WithAuth(func(_ context.Context, _ string) (oauth2.TokenSource, error) {
			return tokenSource, nil
		}),
		zitadelclient.WithGRPCDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				// Route gRPC to the internal target address.
				host, port, _ := net.SplitHostPort(addr)
				if host == cfg.Domain {
					return (&net.Dialer{}).DialContext(ctx, "tcp", cfg.TargetAddr)
				}
				return (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
			}),
		),
	}

	inner, err := zitadelclient.New(ctx, zitadel.New(cfg.Domain, zitadelOpts...), clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating zitadel client: %w", err)
	}

	return &Client{inner: inner}, nil
}

// installDNSOverride overrides net.DefaultResolver so that lookups for domain
// return the targetHost IP. All other lookups use standard resolution.
func installDNSOverride(domain, targetHost, targetPort string) {
	// Resolve targetHost to IP(s) once at startup.
	ips, err := net.LookupHost(targetHost)
	if err != nil || len(ips) == 0 {
		// If we can't resolve targetHost, use it as-is (might already be an IP).
		ips = []string{targetHost}
	}

	_ = targetPort // Port is handled by the SDK's dial target, not DNS.

	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// Use the default dialer for actual DNS queries.
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	}

	// Override the default transport's DialContext to intercept connections to domain.
	origTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		origTransport = &http.Transport{}
	}
	cloned := origTransport.Clone()
	cloned.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, _ := net.SplitHostPort(addr)
		if host == domain {
			// Route to internal target IP.
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ips[0], port))
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	// Wrap the transport to ensure Host header is the bare domain (no port).
	// Zitadel's instance matching only recognizes "zitadel.truvity.xyz",
	// not "zitadel.truvity.xyz:8080".
	http.DefaultTransport = &hostStripPortTransport{domain: domain, inner: cloned}
}

// hostStripPortTransport ensures the Host header is set to just the domain
// (without port) for requests to the target domain. Zitadel's instance matching
// only recognizes the bare domain, not domain:port.
type hostStripPortTransport struct {
	domain string
	inner  http.RoundTripper
}

func (t *hostStripPortTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host, _, _ := net.SplitHostPort(req.URL.Host)
	if host == t.domain {
		clone := req.Clone(req.Context())
		clone.Host = t.domain
		return t.inner.RoundTrip(clone)
	}
	return t.inner.RoundTrip(req)
}

// splitHorizonTokenSource is an oauth2.TokenSource that signs JWT assertions
// with audience=https://domain (matching Zitadel's expectation) and exchanges
// them at the internal token endpoint.
type splitHorizonTokenSource struct {
	signer        jose.Signer
	clientID      string
	audience      []string // ["https://zitadel.truvity.xyz"]
	tokenEndpoint string   // "http://zitadel.truvity.xyz:8080/oauth/v2/token"
	scopes        []string
	httpClient    *http.Client
}

func newSplitHorizonTokenSource(cfg *ClientConfig) (*splitHorizonTokenSource, error) {
	keyFile, err := zitadelclient.ConfigFromKeyFileData(cfg.KeyJSON)
	if err != nil {
		return nil, fmt.Errorf("parsing key file: %w", err)
	}

	signer, err := client.NewSignerFromPrivateKeyByte(keyFile.Key, keyFile.KeyID)
	if err != nil {
		return nil, fmt.Errorf("creating signer: %w", err)
	}

	// The audience is the HTTPS issuer that Zitadel expects.
	audience := "https://" + cfg.Domain

	// The token endpoint is the internal HTTP URL (DNS-hijacked).
	tokenEndpoint := "http://" + cfg.Domain + ":" + cfg.Port + "/oauth/v2/token"

	return &splitHorizonTokenSource{
		signer:        signer,
		clientID:      keyFile.UserID,
		audience:      []string{audience},
		tokenEndpoint: tokenEndpoint,
		scopes:        []string{oidc.ScopeOpenID, zitadelclient.ScopeZitadelAPI()},
		httpClient:    http.DefaultClient, // Uses DefaultTransport (DNS-hijacked)
	}, nil
}

func (s *splitHorizonTokenSource) Token() (*oauth2.Token, error) {
	assertion, err := client.SignedJWTProfileAssertion(s.clientID, s.audience, time.Hour, s.signer)
	if err != nil {
		return nil, fmt.Errorf("signing JWT assertion: %w", err)
	}

	return client.JWTProfileExchange(context.Background(), oidc.NewJWTProfileGrantRequest(assertion, s.scopes...), s)
}

// TokenEndpoint implements the TokenEndpointCaller interface for JWTProfileExchange.
func (s *splitHorizonTokenSource) TokenEndpoint() string {
	return s.tokenEndpoint
}

// HttpClient implements the TokenEndpointCaller interface for JWTProfileExchange.
func (s *splitHorizonTokenSource) HttpClient() *http.Client {
	return s.httpClient
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

// Admin returns the Admin service client (for instance-level operations).
func (c *Client) Admin() admin.AdminServiceClient {
	return c.inner.AdminService()
}

func mustParsePort(port string) uint16 {
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil || p == 0 {
		return 443
	}
	return uint16(p)
}
