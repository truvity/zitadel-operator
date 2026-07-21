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

func TestProject_WithDefaultOrg(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("projtest-%d", time.Now().UnixMilli())

	// Create Project CR using default organization from config.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.ProjectSpec{
			OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}

	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)

	t.Logf("project reconciled: projectId=%s, orgId=%s, ready=%v",
		reconciledProj.Status.ProjectId, reconciledProj.Status.OrganizationId, reconciledProj.Status.Ready)

	if reconciledProj.Status.ProjectId == "" {
		t.Fatal("expected projectId to be set")
	}
	if !reconciledProj.Status.Ready {
		t.Fatal("expected ready=true")
	}

	// Cleanup.
	if err := k8sClient.Delete(ctx, proj); err != nil {
		t.Fatalf("deleting Project CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
	t.Log("project with default org lifecycle complete")
}

func TestProject_WithOrganizationRef(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("projorg-%d", ts)
	projName := fmt.Sprintf("projref-%d", ts)

	// Create Organization CR first.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      orgName,
			Namespace: "default",
		},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization CR: %v", err)
	}

	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)
	t.Logf("org ready: %s (id: %s)", orgName, reconciledOrg.Status.OrganizationId)

	// Create Project CR referencing the Organization.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projName,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.ProjectSpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{
				Name: orgName,
			},
		},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}

	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)

	t.Logf("project reconciled: projectId=%s, orgId=%s", reconciledProj.Status.ProjectId, reconciledProj.Status.OrganizationId)

	// Verify the project's org matches the Organization CR's org.
	if reconciledProj.Status.OrganizationId != reconciledOrg.Status.OrganizationId {
		t.Fatalf("expected orgId=%s, got %s", reconciledOrg.Status.OrganizationId, reconciledProj.Status.OrganizationId)
	}

	// Cleanup (project first, then org).
	if err := k8sClient.Delete(ctx, proj); err != nil {
		t.Fatalf("deleting Project CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)

	if err := k8sClient.Delete(ctx, org); err != nil {
		t.Fatalf("deleting Organization CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	t.Log("project with organizationRef lifecycle complete")
}

func TestProject_WithExplicitOrgId(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("projid-%d", time.Now().UnixMilli())

	// Use the default org ID from config as the explicit org ID.

	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.ProjectSpec{
			OrganizationId: testOrgID,
		},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}

	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)

	if reconciledProj.Status.OrganizationId != testOrgID {
		t.Fatalf("expected orgId=%s, got %s", testOrgID, reconciledProj.Status.OrganizationId)
	}

	// Cleanup.
	if err := k8sClient.Delete(ctx, proj); err != nil {
		t.Fatalf("deleting Project CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
	t.Log("project with explicit orgId lifecycle complete")
}

func TestProject_RolesSync(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("roleproj-%d", time.Now().UnixMilli())

	// Create Project with roles.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.ProjectSpec{
			OrganizationId: testOrgID,
			Roles:          []string{"viewer", "admin", "editor"},
		},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project: %v", err)
	}

	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)
	t.Logf("project with roles ready: %s (id: %s)", name, reconciledProj.Status.ProjectId)

	// Wait a bit for role sync to complete (happens after ensureProject).
	time.Sleep(2 * time.Second)

	// Update: remove "editor", add "operator".
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(proj), proj); err != nil {
		t.Fatalf("getting project: %v", err)
	}
	proj.Spec.Roles = []string{"viewer", "admin", "operator"}
	if err := k8sClient.Update(ctx, proj); err != nil {
		t.Fatalf("updating Project roles: %v", err)
	}

	// Wait for reconciliation.
	time.Sleep(3 * time.Second)

	// Verify still ready.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(proj), &reconciledProj); err != nil {
		t.Fatalf("getting project after role update: %v", err)
	}
	if !reconciledProj.Status.Ready {
		t.Fatal("expected ready=true after role update")
	}
	t.Log("project role sync test passed")

	// Cleanup.
	_ = k8sClient.Delete(ctx, proj)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
}
