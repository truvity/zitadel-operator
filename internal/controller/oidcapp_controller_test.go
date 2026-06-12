package controller

import (
	"testing"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
)

func TestClientIDKey_Default(t *testing.T) {
	cr := &zitadelv1alpha1.OIDCApp{
		Spec: zitadelv1alpha1.OIDCAppSpec{
			SecretRef: zitadelv1alpha1.SecretRefSpec{Name: "test"},
		},
	}

	if got := clientIDKey(cr); got != "client_id" {
		t.Errorf("expected 'client_id', got %q", got)
	}
}

func TestClientIDKey_Custom(t *testing.T) {
	cr := &zitadelv1alpha1.OIDCApp{
		Spec: zitadelv1alpha1.OIDCAppSpec{
			SecretRef: zitadelv1alpha1.SecretRefSpec{
				Name: "test",
				Keys: &zitadelv1alpha1.SecretKeys{ClientId: "my_client_id"},
			},
		},
	}

	if got := clientIDKey(cr); got != "my_client_id" {
		t.Errorf("expected 'my_client_id', got %q", got)
	}
}

func TestClientSecretKey_Default(t *testing.T) {
	cr := &zitadelv1alpha1.OIDCApp{
		Spec: zitadelv1alpha1.OIDCAppSpec{
			SecretRef: zitadelv1alpha1.SecretRefSpec{Name: "test"},
		},
	}

	if got := clientSecretKey(cr); got != "client_secret" {
		t.Errorf("expected 'client_secret', got %q", got)
	}
}

func TestClientSecretKey_Custom(t *testing.T) {
	cr := &zitadelv1alpha1.OIDCApp{
		Spec: zitadelv1alpha1.OIDCAppSpec{
			SecretRef: zitadelv1alpha1.SecretRefSpec{
				Name: "test",
				Keys: &zitadelv1alpha1.SecretKeys{ClientSecret: "oidc_secret"},
			},
		},
	}

	if got := clientSecretKey(cr); got != "oidc_secret" {
		t.Errorf("expected 'oidc_secret', got %q", got)
	}
}
