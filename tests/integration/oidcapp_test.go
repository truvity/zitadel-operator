//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

func TestOIDCApp_WithProjectRef(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("oidcproj-%d", ts)
	appName := fmt.Sprintf("oidcapp-%d", ts)
	secretName := fmt.Sprintf("oidcsecret-%d", ts)

	// Create Project CR.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projName,
			Namespace: "default",
		},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}

	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)
	t.Logf("project ready: %s (id: %s)", projName, reconciledProj.Status.ProjectId)

	// Create OIDCApp CR referencing the project.
	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			ProjectRef: &zitadelv1alpha2.ResourceRef{
				Name: projName,
			},
			Type:       "confidential",
			AuthMethod: "basic",
			RedirectUris: []string{
				"https://app.example.com/callback",
			},
			SecretRef: zitadelv1alpha2.SecretRefSpec{
				Name: secretName,
			},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp CR: %v", err)
	}

	var reconciledApp zitadelv1alpha2.OIDCApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciledApp, 30*time.Second)

	t.Logf("OIDCApp reconciled: appId=%s, clientId=%s, projectId=%s, ready=%v",
		reconciledApp.Status.ApplicationId, reconciledApp.Status.ClientId,
		reconciledApp.Status.ProjectId, reconciledApp.Status.Ready)

	if reconciledApp.Status.ApplicationId == "" {
		t.Fatal("expected applicationId to be set")
	}
	if reconciledApp.Status.ClientId == "" {
		t.Fatal("expected clientId to be set")
	}
	if reconciledApp.Status.ProjectId != reconciledProj.Status.ProjectId {
		t.Fatalf("expected projectId=%s, got %s", reconciledProj.Status.ProjectId, reconciledApp.Status.ProjectId)
	}

	// Verify Secret was created with client credentials.
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, secret); err != nil {
		t.Fatalf("getting Secret: %v", err)
	}
	if len(secret.Data["client_id"]) == 0 {
		t.Fatal("expected client_id in Secret")
	}
	if len(secret.Data["client_secret"]) == 0 {
		t.Fatal("expected client_secret in Secret")
	}
	t.Logf("Secret contains client_id=%s", string(secret.Data["client_id"]))

	// Cleanup (app first, then project).
	if err := k8sClient.Delete(ctx, app); err != nil {
		t.Fatalf("deleting OIDCApp CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(app), &zitadelv1alpha2.OIDCApp{}, 30*time.Second)

	if err := k8sClient.Delete(ctx, proj); err != nil {
		t.Fatalf("deleting Project CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
	t.Log("OIDCApp with projectRef lifecycle complete")
}

func TestOIDCApp_DriftDetection(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("driftproj-%d", ts)
	appName := fmt.Sprintf("driftapp-%d", ts)
	secretName := fmt.Sprintf("driftsecret-%d", ts)

	// Create Project.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project: %v", err)
	}
	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)

	// Create OIDCApp.
	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			ProjectRef:      &zitadelv1alpha2.ResourceRef{Name: projName},
			Type:            "confidential",
			AuthMethod:      "basic",
			RedirectUris:    []string{"https://v1.example.com/cb"},
			AccessTokenType: "bearer",
			SecretRef:       zitadelv1alpha2.SecretRefSpec{Name: secretName},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	var reconciledApp zitadelv1alpha2.OIDCApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciledApp, 30*time.Second)
	t.Logf("initial reconcile: clientId=%s", reconciledApp.Status.ClientId)

	// Update spec: change redirect URIs and token type (triggers drift detection).
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {
		t.Fatalf("getting app for update: %v", err)
	}
	app.Spec.RedirectUris = []string{"https://v2.example.com/cb"}
	app.Spec.AccessTokenType = "jwt"
	if err := k8sClient.Update(ctx, app); err != nil {
		t.Fatalf("updating OIDCApp: %v", err)
	}

	// Wait a bit for reconciliation to process the update.
	time.Sleep(3 * time.Second)

	// Re-fetch and verify status is still ready.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &reconciledApp); err != nil {
		t.Fatalf("getting OIDCApp after update: %v", err)
	}
	if !reconciledApp.Status.Ready {
		t.Fatal("expected ready=true after drift correction")
	}
	t.Logf("drift detection test passed: app still ready after spec update")

	// Cleanup.
	_ = k8sClient.Delete(ctx, app)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(app), &zitadelv1alpha2.OIDCApp{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, proj)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
}
