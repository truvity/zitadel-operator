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

func TestAPIApp_WithProjectRef(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("apiproj-%d", ts)
	appName := fmt.Sprintf("apiapp-%d", ts)
	secretName := fmt.Sprintf("apisecret-%d", ts)

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

	// Create APIApp CR referencing the project.
	app := &zitadelv1alpha2.APIApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.APIAppSpec{
			ProjectRef: &zitadelv1alpha2.ResourceRef{
				Name: projName,
			},
			AuthMethod: "basic",
			SecretRef: zitadelv1alpha2.SecretRefSpec{
				Name: secretName,
			},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating APIApp CR: %v", err)
	}

	var reconciledApp zitadelv1alpha2.APIApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciledApp, 30*time.Second)

	t.Logf("APIApp reconciled: appId=%s, clientId=%s, projectId=%s, ready=%v",
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

	// Verify conditions are set.
	if len(reconciledApp.Status.Conditions) == 0 {
		t.Fatal("expected conditions to be set")
	}

	// Cleanup.
	if err := k8sClient.Delete(ctx, app); err != nil {
		t.Fatalf("deleting APIApp CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(app), &zitadelv1alpha2.APIApp{}, 30*time.Second)

	if err := k8sClient.Delete(ctx, proj); err != nil {
		t.Fatalf("deleting Project CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
	t.Log("APIApp with projectRef lifecycle complete")
}
