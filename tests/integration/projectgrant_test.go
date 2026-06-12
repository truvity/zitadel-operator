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

func TestProjectGrant_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("pgproj-%d", ts)
	orgName := fmt.Sprintf("pgorg-%d", ts)
	grantName := fmt.Sprintf("pgrant-%d", ts)

	// Create Project with a role to grant.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
		Spec: zitadelv1alpha2.ProjectSpec{
			Roles: []string{"viewer"},
		},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project: %v", err)
	}
	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)
	t.Logf("project ready: %s (id: %s)", projName, reconciledProj.Status.ProjectId)

	// Wait for roles to be synced.
	time.Sleep(2 * time.Second)

	// Create Organization to receive the grant.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)
	t.Logf("org ready: %s (id: %s)", orgName, reconciledOrg.Status.OrganizationId)

	// Create ProjectGrant.
	pg := &zitadelv1alpha2.ProjectGrant{
		ObjectMeta: metav1.ObjectMeta{Name: grantName, Namespace: "default"},
		Spec: zitadelv1alpha2.ProjectGrantSpec{
			ProjectRef:    &zitadelv1alpha2.ResourceRef{Name: projName},
			GrantedOrgRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			RoleKeys:      []string{"viewer"},
		},
	}
	if err := k8sClient.Create(ctx, pg); err != nil {
		t.Fatalf("creating ProjectGrant: %v", err)
	}

	var reconciledPG zitadelv1alpha2.ProjectGrant
	waitForReady(t, ctx, client.ObjectKeyFromObject(pg), &reconciledPG, 30*time.Second)

	t.Logf("project grant reconciled: grantId=%s, ready=%v", reconciledPG.Status.GrantId, reconciledPG.Status.Ready)

	if reconciledPG.Status.GrantId == "" {
		t.Fatal("expected grantId to be set")
	}

	// Cleanup (grant → org → project).
	_ = k8sClient.Delete(ctx, pg)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(pg), &zitadelv1alpha2.ProjectGrant{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, proj)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
	t.Log("project grant lifecycle complete")
}
