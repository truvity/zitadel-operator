package controller

import (
	"testing"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
)

func TestZitadelAppType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		crType, crAuth string
		wantApp        applicationv2.OIDCApplicationType
		wantAuth       applicationv2.OIDCAuthMethodType
	}{
		{"confidential", "basic", applicationv2.OIDCApplicationType_OIDC_APP_TYPE_WEB, applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_BASIC},
		{"confidential", "none", applicationv2.OIDCApplicationType_OIDC_APP_TYPE_WEB, applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_NONE},
		{"public", "basic", applicationv2.OIDCApplicationType_OIDC_APP_TYPE_USER_AGENT, applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_NONE},
		{"public", "none", applicationv2.OIDCApplicationType_OIDC_APP_TYPE_USER_AGENT, applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_NONE},
		// The CLI shape: Zitadel applies the RFC 8252 loopback rule to
		// NATIVE only, and a native client never holds a secret.
		{"native", "basic", applicationv2.OIDCApplicationType_OIDC_APP_TYPE_NATIVE, applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_NONE},
		{"native", "none", applicationv2.OIDCApplicationType_OIDC_APP_TYPE_NATIVE, applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_NONE},
	}
	for _, c := range cases {
		gotApp, gotAuth := zitadelAppType(c.crType, c.crAuth)
		if gotApp != c.wantApp || gotAuth != c.wantAuth {
			t.Errorf("%s/%s: got %v/%v, want %v/%v", c.crType, c.crAuth, gotApp, gotAuth, c.wantApp, c.wantAuth)
		}
	}
}
