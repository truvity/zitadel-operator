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

func TestReconciler_ProjectLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a Project CR (cluster-scoped).
	projectName := fmt.Sprintf("envtest-project-%d", time.Now().UnixMilli())
	projectCR := &zitadelv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projectName},
		Spec:       zitadelv1alpha1.ProjectSpec{AssertRolesOnAuth: true},
	}

	if err := k8sClient.Create(ctx, projectCR); err != nil {
		t.Fatalf("create Project CR: %v", err)
	}
	t.Logf("created Project CR: %s", projectName)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = k8sClient.Delete(cleanupCtx, &zitadelv1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: projectName},
		})
		time.Sleep(3 * time.Second)
	})

	// Wait for reconciliation.
	var reconciled zitadelv1alpha1.Project
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: projectName}, &reconciled); err == nil {
			if reconciled.Status.ProjectId != "" {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if reconciled.Status.ProjectId == "" {
		t.Fatalf("project not reconciled within timeout")
	}

	t.Logf("project reconciled: status.projectId=%s, ready=%v", reconciled.Status.ProjectId, reconciled.Status.Ready)

	// Verify in Zitadel.
	getResp, err := zitadelClient.Management().GetProjectByID(ctx, &management.GetProjectByIDRequest{
		Id: reconciled.Status.ProjectId,
	})
	if err != nil {
		t.Fatalf("project not found in Zitadel: %v", err)
	}

	if getResp.GetProject().GetName() != projectName {
		t.Errorf("expected name %q, got %q", projectName, getResp.GetProject().GetName())
	}

	t.Logf("verified in Zitadel: %s", getResp.GetProject().GetName())
}
