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
	"github.com/truvity/zitadel-operator/internal/controller"
)

// TestForeignManager_AdoptionStampsAnnotation covers the owning direction of
// the v0.19 guard: an unannotated tenant CR is adopted — the operator stamps
// its management identity and reconciles normally.
func TestForeignManager_AdoptionStampsAnnotation(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("fmadopt-%d", time.Now().UnixMilli())

	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, proj)
		}
	})

	var reconciled zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciled, 30*time.Second)

	if got, want := reconciled.Annotations[controller.AnnotationManagedBy], cfg.ManagementIdentity(); got != want {
		t.Fatalf("managed-by annotation = %q, want %q", got, want)
	}
	for _, c := range reconciled.Status.Conditions {
		if c.Type == controller.ConditionTypeForeignManager {
			t.Fatalf("owning operator must not carry a ForeignManager condition: %+v", c)
		}
	}
	t.Log("adoption stamped the management identity")
}

// TestForeignManager_ForeignSkipsThenTakeover covers the protective direction:
// a CR annotated for another same-instance operator deployment is not touched
// (ForeignManager condition, no Zitadel project), until ownership is
// transferred by changing the annotation.
func TestForeignManager_ForeignSkipsThenTakeover(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("fmforeign-%d", time.Now().UnixMilli())
	foreignIdentity := cfg.InstanceIdentity() + "/org/000000000000000000"

	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Annotations: map[string]string{
				controller.AnnotationManagedBy: foreignIdentity,
			},
		},
		Spec: zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, proj)
		}
	})

	// Expect the ForeignManager condition; the project must NOT be created.
	deadline := time.Now().Add(20 * time.Second)
	sawForeign := false
	for time.Now().Before(deadline) {
		var cur zitadelv1alpha2.Project
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(proj), &cur); err == nil {
			for _, c := range cur.Status.Conditions {
				if c.Type == controller.ConditionTypeForeignManager && c.Status == metav1.ConditionTrue {
					sawForeign = true
				}
			}
			if sawForeign {
				if cur.Status.ProjectId != "" {
					t.Fatalf("foreign-managed CR must not be reconciled, but got projectId=%s", cur.Status.ProjectId)
				}
				if cur.Status.Ready {
					t.Fatal("foreign-managed CR must not become ready")
				}
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !sawForeign {
		t.Fatal("expected ForeignManager condition on the foreign-annotated CR")
	}

	// Transfer ownership: point the annotation at this operator. The spec
	// bump (assertRolesOnAuth) raises the generation so the transfer is
	// picked up immediately instead of on the periodic requeue.
	var cur zitadelv1alpha2.Project
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(proj), &cur); err != nil {
		t.Fatalf("getting project: %v", err)
	}
	cur.Annotations[controller.AnnotationManagedBy] = cfg.ManagementIdentity()
	cur.Spec.AssertRolesOnAuth = true
	if err := k8sClient.Update(ctx, &cur); err != nil {
		t.Fatalf("transferring ownership: %v", err)
	}

	var reconciled zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciled, 60*time.Second)
	if reconciled.Status.ProjectId == "" {
		t.Fatal("expected the new owner to reconcile the project after takeover")
	}
	for _, c := range reconciled.Status.Conditions {
		if c.Type == controller.ConditionTypeForeignManager {
			t.Fatalf("ForeignManager condition must be released after takeover: %+v", c)
		}
	}
	t.Log("foreign skip and ownership takeover complete")
}
