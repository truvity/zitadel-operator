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

func TestDefaultPrivacyPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("defprivacy-%d", ts)

	// Create with tosLink and privacyLink.
	cr := &zitadelv1alpha2.DefaultPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultPrivacyPolicySpec{
			PrivacyPolicyFields: zitadelv1alpha2.PrivacyPolicyFields{
				TosLink:     "https://example.com/tos",
				PrivacyLink: "https://example.com/privacy",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultPrivacyPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultPrivacyPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Log("DefaultPrivacyPolicy created and ready")

	// Mutate: add helpLink.
	reconciled.Spec.HelpLink = "https://example.com/help"
	if err := k8sClient.Update(ctx, &reconciled); err != nil {
		t.Fatalf("updating DefaultPrivacyPolicy: %v", err)
	}
	time.Sleep(3 * time.Second)

	var updated zitadelv1alpha2.DefaultPrivacyPolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &updated); err != nil {
		t.Fatalf("getting updated DefaultPrivacyPolicy: %v", err)
	}
	if !updated.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}
	t.Log("DefaultPrivacyPolicy mutated successfully")

	// Delete.
	if err := k8sClient.Delete(ctx, &updated); err != nil {
		t.Fatalf("deleting DefaultPrivacyPolicy: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultPrivacyPolicy{}, 30*time.Second)
	t.Log("DefaultPrivacyPolicy deleted (reset to defaults)")
}

func TestPrivacyPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("privpol-org-%d", ts)
	policyName := fmt.Sprintf("privpol-%d", ts)

	// Create org.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)
	t.Logf("Organization ready: %s", reconciledOrg.Status.OrganizationId)

	// Create PrivacyPolicy with orgRef.
	cr := &zitadelv1alpha2.PrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: zitadelv1alpha2.PrivacyPolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			PrivacyPolicyFields: zitadelv1alpha2.PrivacyPolicyFields{
				TosLink:      "https://example.com/tos",
				PrivacyLink:  "https://example.com/privacy",
				SupportEmail: "support@example.com",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating PrivacyPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.PrivacyPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	if reconciled.Status.OrganizationId == "" {
		t.Fatal("expected organizationId in status")
	}
	t.Logf("PrivacyPolicy ready, orgId=%s", reconciled.Status.OrganizationId)

	// Mutate: add HelpLink.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	cr.Spec.HelpLink = "https://example.com/help"
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Delete (resets to default).
	if err := k8sClient.Delete(ctx, &reconciled); err != nil {
		t.Fatalf("deleting PrivacyPolicy: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.PrivacyPolicy{}, 30*time.Second)
	t.Log("PrivacyPolicy deleted (reset to default)")

	// Cleanup org.
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
}

func TestDefaultOIDCSettings_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("defoidc-%d", ts)

	// Create with accessTokenLifetime="6h".
	cr := &zitadelv1alpha2.DefaultOIDCSettings{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultOIDCSettingsSpec{
			OIDCSettingsFields: zitadelv1alpha2.OIDCSettingsFields{
				AccessTokenLifetime: "6h",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultOIDCSettings: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultOIDCSettings
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Log("DefaultOIDCSettings created and ready with accessTokenLifetime=6h")

	// Mutate to "8h".
	reconciled.Spec.AccessTokenLifetime = "8h"
	if err := k8sClient.Update(ctx, &reconciled); err != nil {
		t.Fatalf("updating DefaultOIDCSettings: %v", err)
	}
	time.Sleep(3 * time.Second)

	var updated zitadelv1alpha2.DefaultOIDCSettings
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &updated); err != nil {
		t.Fatalf("getting updated DefaultOIDCSettings: %v", err)
	}
	if !updated.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}
	t.Log("DefaultOIDCSettings mutated to 8h successfully")

	// Delete (resets to 12h default).
	if err := k8sClient.Delete(ctx, &updated); err != nil {
		t.Fatalf("deleting DefaultOIDCSettings: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultOIDCSettings{}, 30*time.Second)
	t.Log("DefaultOIDCSettings deleted (reset to 12h defaults)")
}
