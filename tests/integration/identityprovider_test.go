//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

func TestIdentityProvider_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("idp-%d", ts)

	// Create IdentityProvider CR using default organization from config.
	idpCR := &zitadelv1alpha2.IdentityProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.IdentityProviderSpec{
			OrganizationId: testOrgID,
			Name:           fmt.Sprintf("Test OIDC IDP %d", ts),
			Issuer:         "https://accounts.google.com",
			ClientId:       fmt.Sprintf("test-client-%d", ts),
			ClientSecret:   "test-secret-value",
			Scopes:         []string{"openid", "profile", "email"},
		},
	}
	if err := k8sClient.Create(ctx, idpCR); err != nil {
		t.Fatalf("creating IdentityProvider: %v", err)
	}

	var reconciled zitadelv1alpha2.IdentityProvider
	waitForReady(t, ctx, client.ObjectKeyFromObject(idpCR), &reconciled, 30*time.Second)

	t.Logf("identityprovider reconciled: idpId=%s, ready=%v", reconciled.Status.IdpId, reconciled.Status.Ready)

	if reconciled.Status.IdpId == "" {
		t.Fatal("expected idpId to be set")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, idpCR)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(idpCR), &zitadelv1alpha2.IdentityProvider{}, 30*time.Second)
	t.Log("identityprovider lifecycle complete")
}
