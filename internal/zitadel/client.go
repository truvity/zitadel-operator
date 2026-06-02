// Package zitadel provides a client wrapper around the official zitadel-go SDK
// for use by the operator controllers.
package zitadel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"

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
	// When set, the SDK uses Domain for Host/:authority headers but dials TargetAddr.
	// When empty, the SDK dials Domain directly.
	TargetAddr string
}

// NewClient creates a new Zitadel client using JWT Profile authentication.
// If cfg.TargetAddr is set, traffic is routed to the internal address while
// presenting cfg.Domain as the Host header (for Zitadel instance matching).
func NewClient(ctx context.Context, cfg *ClientConfig) (*Client, error) {
	keyFile, err := zitadelclient.ConfigFromKeyFileData(cfg.KeyJSON)
	if err != nil {
		return nil, fmt.Errorf("parsing key file: %w", err)
	}

	var zitadelOpts []zitadel.Option
	switch {
	case cfg.TargetAddr != "":
		// Use insecure mode so gRPC transport works without TLS.
		// The issuer mismatch (http://domain:port vs https://domain) is handled
		// by rewriting the OIDC discovery response in the transport layer.
		zitadelOpts = append(zitadelOpts, zitadel.WithInsecure(cfg.Port))
	case cfg.InsecurePlaintext:
		zitadelOpts = append(zitadelOpts, zitadel.WithInsecure(cfg.Port))
	default:
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

	// When TargetAddr is set, route gRPC and HTTP to the internal address
	// while keeping the external domain for Host/:authority matching.
	if cfg.TargetAddr != "" {
		// gRPC: dial the internal target instead of the domain.
		clientOpts = append(clientOpts,
			zitadelclient.WithGRPCDialOptions(
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "tcp", cfg.TargetAddr)
				}),
			),
		)

		// HTTP: The zitadel/oidc library uses http.DefaultClient for OIDC
		// discovery and does NOT read oauth2.HTTPClient from context.
		// Override http.DefaultTransport to route all HTTP traffic from this
		// process to the internal target while setting Host to the external domain.
		// This is safe because this is a single-purpose operator binary.
		//
		// The SDK uses Origin = http://domain:port (due to WithInsecure), but
		// Zitadel returns https:// URLs in its discovery response. We rewrite
		// ALL https://domain references to http://domain:port so issuer validation
		// and JWT audience match.
		expectedOrigin := "http://" + cfg.Domain + ":" + cfg.Port
		http.DefaultTransport = &hostOverrideTransport{
			externalDomain:  cfg.Domain,
			expectedOrigin:  expectedOrigin,
			canonicalOrigin: "https://" + cfg.Domain,
			inner: &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, network, cfg.TargetAddr)
				},
			},
		}
	}

	inner, err := zitadelclient.New(ctx, zitadel.New(cfg.Domain, zitadelOpts...), clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating zitadel client: %w", err)
	}

	return &Client{inner: inner}, nil
}

// hostOverrideTransport ensures the Host header is set to the external domain
// on every HTTP request, and rewrites response bodies to replace the canonical
// HTTPS origin with the expected HTTP origin. This ensures OIDC discovery issuer
// validation passes and JWT audience matches.
type hostOverrideTransport struct {
	externalDomain  string // e.g., "zitadel.truvity.xyz"
	expectedOrigin  string // what the SDK expects, e.g., "http://zitadel.truvity.xyz:8080"
	canonicalOrigin string // what Zitadel returns, e.g., "https://zitadel.truvity.xyz"
	inner           http.RoundTripper
}

func (t *hostOverrideTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Host = t.externalDomain
	resp, err := t.inner.RoundTrip(clone)
	if err != nil {
		return nil, err
	}
	// Rewrite discovery/token responses: replace https://domain with
	// http://domain:port so issuer validation and JWT audience both match.
	if resp.Header.Get("Content-Type") == "application/json" {
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		body = bytes.ReplaceAll(body, []byte(t.canonicalOrigin), []byte(t.expectedOrigin))
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
	}
	return resp, nil
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
