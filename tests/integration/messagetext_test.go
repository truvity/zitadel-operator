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

// --- DefaultMessageText ---

func TestDefaultMessageText_Init_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("defmsgtext-init-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.DefaultMessageText{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultMessageTextSpec{
			MessageTextFields: zitadelv1alpha2.MessageTextFields{
				Type:       "init",
				Language:   "en",
				Title:      "Custom Init Title",
				Subject:    "Custom Init Subject",
				Greeting:   "Hello {{.FirstName}}!",
				Text:       "Custom init body text",
				ButtonText: "Initialize",
				FooterText: "Operator Test Footer",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultMessageText: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultMessageText
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("DefaultMessageText reconciled: ready=%v", reconciled.Status.Ready)

	// Mutate: change Subject.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	cr.Spec.Subject = "Updated Init Subject"
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

	// Cleanup (resets to default).
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultMessageText{}, 30*time.Second)
	t.Log("DefaultMessageText init lifecycle complete")
}

func TestDefaultMessageText_PasswordReset_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("defmsgtext-pwreset-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.DefaultMessageText{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultMessageTextSpec{
			MessageTextFields: zitadelv1alpha2.MessageTextFields{
				Type:       "passwordReset",
				Language:   "en",
				Title:      "Reset Your Password",
				Subject:    "Password Reset Request",
				ButtonText: "Reset Password",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultMessageText: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultMessageText
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("DefaultMessageText passwordReset reconciled: ready=%v", reconciled.Status.Ready)

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultMessageText{}, 30*time.Second)
	t.Log("DefaultMessageText passwordReset lifecycle complete")
}

// --- MessageText (org-scoped) ---

func TestMessageText_Init_Lifecycle(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixMilli()

	// Create org first.
	orgName := fmt.Sprintf("msgtext-org-%d", suffix)
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
		Spec: zitadelv1alpha2.OrganizationSpec{
			Name: orgName,
		},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create MessageText with orgRef.
	name := fmt.Sprintf("msgtext-init-%d", suffix)
	cr := &zitadelv1alpha2.MessageText{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.MessageTextSpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			MessageTextFields: zitadelv1alpha2.MessageTextFields{
				Type:       "init",
				Language:   "en",
				Title:      "Org Custom Init Title",
				Subject:    "Org Custom Init Subject",
				Greeting:   "Welcome {{.FirstName}} to the org!",
				Text:       "Org custom init body",
				ButtonText: "Get Started",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating MessageText: %v", err)
	}

	var reconciled zitadelv1alpha2.MessageText
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("MessageText reconciled: ready=%v, orgId=%s", reconciled.Status.Ready, reconciled.Status.OrganizationId)

	if reconciled.Status.OrganizationId == "" {
		t.Fatal("expected non-empty organizationId in status")
	}

	// Mutate: change Greeting.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	cr.Spec.Greeting = "Updated greeting {{.FirstName}}!"
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

	// Cleanup: delete message text first (resets to default), then org.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.MessageText{}, 30*time.Second)

	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	t.Log("MessageText init lifecycle complete")
}
