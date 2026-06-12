//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
)

func TestReconciler_OIDCAppLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixMilli())

	// Step 1: Create a Project first (OIDCApp needs a project).
	projectName := "oidctest-project-" + suffix
	projectCR := &zitadelv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projectName},
		Spec:       zitadelv1alpha1.ProjectSpec{AssertRolesOnAuth: true},
	}
	if err := k8sClient.Create(ctx, projectCR); err != nil {
		t.Fatalf("create Project CR: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = k8sClient.Delete(cleanupCtx, &zitadelv1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: projectName},
		})
		time.Sleep(3 * time.Second)
	})

	// Wait for project ready.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var p zitadelv1alpha1.Project
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: projectName}, &p); err == nil {
			if p.Status.ProjectId != "" {
				t.Logf("project ready: %s (id: %s)", projectName, p.Status.ProjectId)
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Step 2: Create OIDCApp CR.
	appName := "oidctest-app-" + suffix
	appCR := &zitadelv1alpha1.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: "default",
		},
		Spec: zitadelv1alpha1.OIDCAppSpec{
			Project:                  projectName,
			Type:                     "public",
			AuthMethod:               "none",
			RedirectUris:             []string{"http://localhost:8000"},
			PostLogoutRedirectUris:   []string{"http://localhost:8000"},
			AccessTokenType:          "bearer",
			AccessTokenRoleAssertion: true,
			IdTokenRoleAssertion:     true,
			SecretRef: zitadelv1alpha1.SecretRefSpec{
				Name: appName + "-secret",
			},
		},
	}

	if err := k8sClient.Create(ctx, appCR); err != nil {
		t.Fatalf("create OIDCApp CR: %v", err)
	}
	t.Logf("created OIDCApp CR: %s", appName)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = k8sClient.Delete(cleanupCtx, &zitadelv1alpha1.OIDCApp{
			ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"},
		})
		time.Sleep(3 * time.Second)
	})

	// Step 3: Wait for OIDCApp reconciliation.
	var reconciledApp zitadelv1alpha1.OIDCApp
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: appName, Namespace: "default"}, &reconciledApp); err == nil {
			if reconciledApp.Status.ClientId != "" {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if reconciledApp.Status.ClientId == "" {
		t.Fatalf("OIDCApp not reconciled within timeout")
	}

	t.Logf("OIDCApp reconciled: clientId=%s, ready=%v", reconciledApp.Status.ClientId, reconciledApp.Status.Ready)

	// Step 4: Verify app in Zitadel.
	var reconciledProject zitadelv1alpha1.Project
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: projectName}, &reconciledProject); err != nil {
		t.Fatalf("get project CR: %v", err)
	}

	listResp, err := zitadelClient.Management().ListApps(ctx, &management.ListAppsRequest{
		ProjectId: reconciledProject.Status.ProjectId,
	})
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}

	var found bool
	for _, app := range listResp.GetResult() {
		if app.GetName() == appName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("app %q not found in Zitadel", appName)
	}

	t.Log("verified app exists in Zitadel")

	// Step 5: Update spec → drift detection.
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: appName, Namespace: "default"}, &reconciledApp); err != nil {
		t.Fatalf("get OIDCApp for update: %v", err)
	}

	reconciledApp.Spec.RedirectUris = []string{"http://localhost:8000", "http://localhost:9000"}
	reconciledApp.Spec.AccessTokenType = "jwt"
	if err := k8sClient.Update(ctx, &reconciledApp); err != nil {
		t.Fatalf("update OIDCApp CR: %v", err)
	}

	t.Log("updated OIDCApp spec (added redirect URI, changed token type to jwt)")

	// Wait for reconciler to process drift.
	time.Sleep(5 * time.Second)
	t.Log("OIDCApp lifecycle test complete")
}
