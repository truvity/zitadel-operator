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

func TestProjectGrantMember_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("pgmproj-%d", ts)
	grantedOrgName := fmt.Sprintf("pgmorg-%d", ts)
	muName := fmt.Sprintf("pgmmu-%d", ts)
	userName := fmt.Sprintf("pgmuser-%d", ts)
	muSecretName := fmt.Sprintf("pgmmusecret-%d", ts)
	grantName := fmt.Sprintf("pgmgrant-%d", ts)
	memberName := fmt.Sprintf("pgmmember-%d", ts)

	// Create a Project.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
		Spec:       zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project: %v", err)
	}
	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)

	// Create a second Organization (to grant the project to).
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: grantedOrgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create a MachineUser in the granted org.
	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{Name: muName, Namespace: "default"},
		Spec: zitadelv1alpha2.MachineUserSpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: grantedOrgName},
			UserName:        userName,
			KeySecretRef:    zitadelv1alpha2.MachineKeySecretRef{Name: muSecretName},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser: %v", err)
	}
	var reconciledMU zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciledMU, 30*time.Second)

	// Create a ProjectGrant.
	grant := &zitadelv1alpha2.ProjectGrant{
		ObjectMeta: metav1.ObjectMeta{Name: grantName, Namespace: "default"},
		Spec: zitadelv1alpha2.ProjectGrantSpec{
			OrganizationId: testOrgID,
			ProjectRef:     &zitadelv1alpha2.ResourceRef{Name: projName},
			GrantedOrgRef:  &zitadelv1alpha2.ResourceRef{Name: grantedOrgName},
			RoleKeys:       []string{},
		},
	}
	if err := k8sClient.Create(ctx, grant); err != nil {
		t.Fatalf("creating ProjectGrant: %v", err)
	}
	var reconciledGrant zitadelv1alpha2.ProjectGrant
	waitForReady(t, ctx, client.ObjectKeyFromObject(grant), &reconciledGrant, 30*time.Second)
	t.Logf("grant ready: grantId=%s", reconciledGrant.Status.GrantId)

	// Create ProjectGrantMember.
	member := &zitadelv1alpha2.ProjectGrantMember{
		ObjectMeta: metav1.ObjectMeta{Name: memberName, Namespace: "default"},
		Spec: zitadelv1alpha2.ProjectGrantMemberSpec{
			OrganizationId: testOrgID,
			ProjectRef:     &zitadelv1alpha2.ResourceRef{Name: projName},
			GrantId:        reconciledGrant.Status.GrantId,
			UserRef:        &zitadelv1alpha2.ResourceRef{Name: muName},
			Roles:          []string{"PROJECT_GRANT_OWNER"},
		},
	}
	if err := k8sClient.Create(ctx, member); err != nil {
		t.Fatalf("creating ProjectGrantMember: %v", err)
	}
	var reconciledMember zitadelv1alpha2.ProjectGrantMember
	waitForReady(t, ctx, client.ObjectKeyFromObject(member), &reconciledMember, 30*time.Second)
	t.Logf("ProjectGrantMember reconciled: ready=%v", reconciledMember.Status.Ready)

	// Cleanup (reverse order).
	_ = k8sClient.Delete(ctx, member)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(member), &zitadelv1alpha2.ProjectGrantMember{}, 30*time.Second)

	_ = k8sClient.Delete(ctx, grant)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(grant), &zitadelv1alpha2.ProjectGrant{}, 30*time.Second)

	_ = k8sClient.Delete(ctx, mu)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(mu), &zitadelv1alpha2.MachineUser{}, 30*time.Second)

	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)

	_ = k8sClient.Delete(ctx, proj)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)

	t.Log("ProjectGrantMember lifecycle complete")
}
