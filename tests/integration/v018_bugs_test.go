//go:build integration

package integration

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
)

// TestINF430_AdoptedConfidentialApp_SecretRegenerated (S-260) reproduces
// INF-430: a confidential app that already exists in Zitadel (created outside
// the operator) is adopted by a matching CR; its client secret cannot be read
// back, so when the referenced Secret does not hold one, the operator must
// regenerate it and store a complete credential set.
func TestINF430_AdoptedConfidentialApp_SecretRegenerated(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("v018-adopt-proj-%d", ts)
	appName := fmt.Sprintf("v018-adopt-app-%d", ts)
	secretName := fmt.Sprintf("v018-adopt-secret-%d", ts)

	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
		Spec:       zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, proj) })
	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 60*time.Second)

	// Pre-create the confidential app directly in Zitadel (out-of-band /
	// Pulumi-managed state) using the CR's display name.
	createResp, err := zitadelClient.Application().CreateApplication(ctx, &applicationv2.CreateApplicationRequest{
		ProjectId: reconciledProj.Status.ProjectId,
		Name:      appName,
		ApplicationType: &applicationv2.CreateApplicationRequest_OidcConfiguration{
			OidcConfiguration: &applicationv2.CreateOIDCApplicationRequest{
				RedirectUris:    []string{"https://v018-adopt.example.com/cb"},
				ResponseTypes:   []applicationv2.OIDCResponseType{applicationv2.OIDCResponseType_OIDC_RESPONSE_TYPE_CODE},
				GrantTypes:      []applicationv2.OIDCGrantType{applicationv2.OIDCGrantType_OIDC_GRANT_TYPE_AUTHORIZATION_CODE},
				ApplicationType: applicationv2.OIDCApplicationType_OIDC_APP_TYPE_WEB,
				AuthMethodType:  applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_BASIC,
			},
		},
	})
	if err != nil {
		t.Fatalf("pre-creating app in Zitadel: %v", err)
	}
	preExistingAppID := createResp.GetApplicationId()

	// The adopting CR: same display name, no Secret exists yet.
	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			ProjectRef:   &zitadelv1alpha2.ResourceRef{Name: projName},
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://v018-adopt.example.com/cb"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: secretName},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, app) })

	var reconciled zitadelv1alpha2.OIDCApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciled, 60*time.Second)

	if reconciled.Status.ApplicationId != preExistingAppID {
		t.Fatalf("expected adoption of app %s, got %s (duplicate created?)", preExistingAppID, reconciled.Status.ApplicationId)
	}

	// INF-430: the Secret must hold BOTH client_id and a regenerated
	// client_secret.
	var sec corev1.Secret
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, &sec); err != nil {
		t.Fatalf("reading credential Secret: %v", err)
	}
	if len(sec.Data["client_id"]) == 0 {
		t.Fatal("adopted app Secret missing client_id")
	}
	if len(sec.Data["client_secret"]) == 0 {
		t.Fatal("INF-430: adopted confidential app Secret missing regenerated client_secret")
	}
	regenerated := string(sec.Data["client_secret"])

	// Idempotency: a re-reconcile must NOT rotate the secret again.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &reconciled); err != nil {
		t.Fatal(err)
	}
	reconciled.Spec.RedirectUris = append(reconciled.Spec.RedirectUris, "https://v018-adopt2.example.com/cb")
	if err := k8sClient.Update(ctx, &reconciled); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, &sec); err != nil {
		t.Fatal(err)
	}
	if string(sec.Data["client_secret"]) != regenerated {
		t.Fatal("client_secret rotated on a routine reconcile (needless churn)")
	}
	t.Log("INF-430: adopted confidential app got a regenerated client_secret exactly once")
}

// TestINF400_URIListUpdate_NoStaleState (S-261) is the targeted repro for
// INF-400 (reported stale URI-list updates): successive redirect-URI list
// mutations (append, replace, shrink) must each converge server-side with no
// stale entries.
func TestINF400_URIListUpdate_NoStaleState(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("v018-inf400-proj-%d", ts)
	appName := fmt.Sprintf("v018-inf400-app-%d", ts)

	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
		Spec:       zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, proj) })

	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			ProjectRef:   &zitadelv1alpha2.ResourceRef{Name: projName},
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://a.v018-inf400.example.com/cb"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: fmt.Sprintf("v018-inf400-secret-%d", ts)},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, app) })

	var reconciled zitadelv1alpha2.OIDCApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciled, 60*time.Second)

	serverURIs := func() []string {
		resp, err := zitadelClient.Application().GetApplication(ctx, &applicationv2.GetApplicationRequest{
			ApplicationId: reconciled.Status.ApplicationId,
		})
		if err != nil {
			t.Fatalf("reading app: %v", err)
		}
		uris := append([]string(nil), resp.GetApplication().GetOidcConfiguration().GetRedirectUris()...)
		slices.Sort(uris)
		return uris
	}

	steps := [][]string{
		// append
		{"https://a.v018-inf400.example.com/cb", "https://b.v018-inf400.example.com/cb"},
		// replace
		{"https://c.v018-inf400.example.com/cb", "https://d.v018-inf400.example.com/cb"},
		// shrink
		{"https://c.v018-inf400.example.com/cb"},
	}
	for i, want := range steps {
		var cur zitadelv1alpha2.OIDCApp
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &cur); err != nil {
			t.Fatal(err)
		}
		cur.Spec.RedirectUris = append([]string(nil), want...)
		if err := k8sClient.Update(ctx, &cur); err != nil {
			t.Fatalf("step %d update: %v", i, err)
		}

		wantSorted := append([]string(nil), want...)
		slices.Sort(wantSorted)
		deadline := time.Now().Add(30 * time.Second)
		for {
			got := serverURIs()
			if slices.Equal(got, wantSorted) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("INF-400 REPRO at step %d: server URIs %v never converged to %v (stale list)", i, got, wantSorted)
			}
			time.Sleep(1 * time.Second)
		}
		t.Logf("step %d converged: %v", i, wantSorted)
	}
	t.Log("INF-400: no stale URI-list state reproduced — every mutation converged")
}
