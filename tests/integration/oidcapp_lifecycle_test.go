//go:build integration

package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/controller"
)

func TestReconciler_OIDCAppLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Start envtest.
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	restCfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() { _ = testEnv.Stop() })

	if err := zitadelv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme:                 scheme.Scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	})
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	// Register both Project and OIDCApp controllers.
	if err := (&controller.ProjectReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		t.Fatalf("setup ProjectReconciler: %v", err)
	}

	if err := (&controller.OIDCAppReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		t.Fatalf("setup OIDCAppReconciler: %v", err)
	}

	mgrCtx, mgrCancel := context.WithCancel(ctx)
	defer mgrCancel()
	go func() { _ = mgr.Start(mgrCtx) }()

	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("cache sync timeout")
	}

	k8sClient, err := client.New(restCfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatalf("create k8s client: %v", err)
	}

	// Step 1: Create a Project first (OIDCApp needs a project).
	suffix := fmt.Sprintf("%d", time.Now().UnixMilli())
	projectName := "oidctest-project-" + suffix

	projectCR := &zitadelv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projectName},
		Spec:       zitadelv1alpha1.ProjectSpec{AssertRolesOnAuth: true},
	}
	if err := k8sClient.Create(ctx, projectCR); err != nil {
		t.Fatalf("create Project CR: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), &zitadelv1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: projectName},
		})
		time.Sleep(3 * time.Second)
	})

	// Wait for project to be reconciled.
	waitForProjectReady(t, ctx, k8sClient, projectName)

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
		_ = k8sClient.Delete(context.Background(), &zitadelv1alpha1.OIDCApp{
			ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"},
		})
		time.Sleep(3 * time.Second)
	})

	// Step 3: Wait for OIDCApp to be reconciled.
	var reconciledApp zitadelv1alpha1.OIDCApp
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: appName, Namespace: "default"}, &reconciledApp); err == nil {
			if reconciledApp.Status.ClientId != "" {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if reconciledApp.Status.ClientId == "" {
		t.Fatalf("OIDCApp not reconciled within timeout (status.clientId is empty)")
	}

	t.Logf("OIDCApp reconciled: clientId=%s, ready=%v", reconciledApp.Status.ClientId, reconciledApp.Status.Ready)

	// Step 4: Verify app exists in Zitadel via project listing.
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
			t.Logf("verified app exists in Zitadel: %s (clientId from API)", app.GetName())
			break
		}
	}
	if !found {
		t.Fatalf("app %q not found in Zitadel project %s", appName, reconciledProject.Status.ProjectId)
	}

	// Step 5: Update spec to trigger drift detection.
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: appName, Namespace: "default"}, &reconciledApp); err != nil {
		t.Fatalf("get OIDCApp CR for update: %v", err)
	}

	reconciledApp.Spec.RedirectUris = []string{"http://localhost:8000", "http://localhost:9000"}
	reconciledApp.Spec.AccessTokenType = "jwt"
	if err := k8sClient.Update(ctx, &reconciledApp); err != nil {
		t.Fatalf("update OIDCApp CR: %v", err)
	}

	t.Log("updated OIDCApp spec (added redirect URI, changed access token type to jwt)")

	// Wait for reconciler to process update.
	time.Sleep(5 * time.Second)

	t.Log("OIDCApp lifecycle test complete")
}

func waitForProjectReady(t *testing.T, ctx context.Context, k8sClient client.Client, name string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var p zitadelv1alpha1.Project
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &p); err == nil {
			if p.Status.ProjectId != "" {
				t.Logf("project ready: %s (id: %s)", name, p.Status.ProjectId)
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("project %s not ready within timeout", name)
}
