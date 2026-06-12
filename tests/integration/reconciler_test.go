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

func TestReconciler_ProjectLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start envtest (lightweight kube-apiserver + etcd).
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	restCfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}

	t.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			t.Logf("stop envtest: %v", err)
		}
	})

	// Register CRDs with scheme.
	if err := zitadelv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	// Create controller-runtime manager.
	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable metrics
		},
		HealthProbeBindAddress: "0", // disable health probes
	})
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	// Register Project controller with real Zitadel client.
	if err := (&controller.ProjectReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		t.Fatalf("setup ProjectReconciler: %v", err)
	}

	// Start manager in background.
	mgrCtx, mgrCancel := context.WithCancel(ctx)
	defer mgrCancel()

	go func() {
		if err := mgr.Start(mgrCtx); err != nil {
			t.Logf("manager stopped: %v", err)
		}
	}()

	// Wait for cache sync.
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("cache sync timeout")
	}

	// Create a k8s client for test assertions.
	k8sClient, err := client.New(restCfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatalf("create k8s client: %v", err)
	}

	// Create a Project CR (cluster-scoped — no namespace).
	projectName := fmt.Sprintf("envtest-project-%d", time.Now().UnixMilli())
	projectCR := &zitadelv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: projectName,
		},
		Spec: zitadelv1alpha1.ProjectSpec{
			AssertRolesOnAuth: true,
		},
	}

	if err := k8sClient.Create(ctx, projectCR); err != nil {
		t.Fatalf("create Project CR: %v", err)
	}

	t.Logf("created Project CR: %s", projectName)

	t.Cleanup(func() {
		// Delete the CR (triggers finalizer → Zitadel delete).
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if err := k8sClient.Delete(cleanupCtx, &zitadelv1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: projectName},
		}); err != nil {
			t.Logf("cleanup delete CR: %v", err)
		}

		// Wait for reconciler to process deletion.
		time.Sleep(3 * time.Second)
	})

	// Wait for the controller to reconcile and set status.projectId.
	var reconciled zitadelv1alpha1.Project

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: projectName}, &reconciled); err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if reconciled.Status.ProjectId != "" {
			break
		}

		time.Sleep(500 * time.Millisecond)
	}

	if reconciled.Status.ProjectId == "" {
		t.Fatalf("project was not reconciled within timeout (status.projectId is empty)")
	}

	t.Logf("project reconciled: status.projectId=%s, ready=%v", reconciled.Status.ProjectId, reconciled.Status.Ready)

	// Verify the project exists in Zitadel.
	getResp, err := zitadelClient.Management().GetProjectByID(ctx, &management.GetProjectByIDRequest{
		Id: reconciled.Status.ProjectId,
	})
	if err != nil {
		t.Fatalf("project not found in Zitadel: %v", err)
	}

	t.Logf("verified project exists in Zitadel: %s (name: %s)", reconciled.Status.ProjectId, getResp.GetProject().GetName())

	if getResp.GetProject().GetName() != projectName {
		t.Errorf("expected project name %q in Zitadel, got %q", projectName, getResp.GetProject().GetName())
	}
}
