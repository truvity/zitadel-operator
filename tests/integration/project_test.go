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
		Spec: zitadelv1alpha2.ProjectSpec{},
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
	if cfg.DefaultOrganizationId == "" {
		t.Skip("no defaultOrganizationId in config — cannot test explicit orgId")
	}

	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.ProjectSpec{
			OrganizationId: cfg.DefaultOrganizationId,
		},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}

	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)

	if reconciledProj.Status.OrganizationId != cfg.DefaultOrganizationId {
		t.Fatalf("expected orgId=%s, got %s", cfg.DefaultOrganizationId, reconciledProj.Status.OrganizationId)
	}

	// Cleanup.
	if err := k8sClient.Delete(ctx, proj); err != nil {
		t.Fatalf("deleting Project CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
	t.Log("project with explicit orgId lifecycle complete")
}
