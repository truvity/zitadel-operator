// Package zitadel provides a client wrapper around the official zitadel-go SDK
// for use by the operator controllers.
package zitadel

import (
	"context"
	"fmt"
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

// NewClient creates a new Zitadel client using JWT Profile authentication.
func NewClient(ctx context.Context, domain, port string, keyJSON []byte, insecurePlaintext bool) (*Client, error) {
	keyFile, err := zitadelclient.ConfigFromKeyFileData(keyJSON)
	if err != nil {
		return nil, fmt.Errorf("parsing key file: %w", err)
	}

	var zitadelOpts []zitadel.Option
	if insecurePlaintext {
		zitadelOpts = append(zitadelOpts, zitadel.WithInsecure(port))
	} else {
		zitadelOpts = append(zitadelOpts, zitadel.WithPort(mustParsePort(port)))
	}

	clientOpts := []zitadelclient.Option{
		zitadelclient.WithAuth(zitadelclient.AuthenticationJWTProfile(
			keyFile,
			oidc.ScopeOpenID,
			zitadelclient.ScopeZitadelAPI(),
		)),
	}
	if insecurePlaintext {
		clientOpts = append(clientOpts,
			zitadelclient.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
	}

	inner, err := zitadelclient.New(ctx, zitadel.New(domain, zitadelOpts...), clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating zitadel client: %w", err)
	}

	return &Client{inner: inner}, nil
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
