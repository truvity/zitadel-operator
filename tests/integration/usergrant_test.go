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

func TestUserGrant_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("ugproj-%d", ts)
	muName := fmt.Sprintf("ugmu-%d", ts)
	userName := fmt.Sprintf("ugbot-%d", ts)
	secretName := fmt.Sprintf("ugkey-%d", ts)
	grantName := fmt.Sprintf("uggrant-%d", ts)

	// Create Project.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
		Spec:       zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project: %v", err)
	}
	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)
	t.Logf("project ready: %s (id: %s)", projName, reconciledProj.Status.ProjectId)

	// Create MachineUser.
	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{Name: muName, Namespace: "default"},
		Spec: zitadelv1alpha2.MachineUserSpec{
			OrganizationId: testOrgID,
			UserName:       userName,
			KeySecretRef:   zitadelv1alpha2.MachineKeySecretRef{Name: secretName},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser: %v", err)
	}
	var reconciledMU zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciledMU, 30*time.Second)
	t.Logf("machine user ready: %s (id: %s)", muName, reconciledMU.Status.UserId)

	// Create UserGrant referencing both.
	grant := &zitadelv1alpha2.UserGrant{
		ObjectMeta: metav1.ObjectMeta{Name: grantName, Namespace: "default"},
		Spec: zitadelv1alpha2.UserGrantSpec{
			OrganizationId: testOrgID,
			UserRef:        &zitadelv1alpha2.ResourceRef{Name: muName},
			ProjectRef:     &zitadelv1alpha2.ResourceRef{Name: projName},
			RoleKeys:       []string{},
		},
	}
	if err := k8sClient.Create(ctx, grant); err != nil {
		t.Fatalf("creating UserGrant: %v", err)
	}

	var reconciledGrant zitadelv1alpha2.UserGrant
	waitForReady(t, ctx, client.ObjectKeyFromObject(grant), &reconciledGrant, 30*time.Second)

	t.Logf("user grant reconciled: grantId=%s, ready=%v", reconciledGrant.Status.GrantId, reconciledGrant.Status.Ready)

	if reconciledGrant.Status.GrantId == "" {
		t.Fatal("expected grantId to be set")
	}

	// Cleanup (grant → machine user → project).
	_ = k8sClient.Delete(ctx, grant)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(grant), &zitadelv1alpha2.UserGrant{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, mu)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(mu), &zitadelv1alpha2.MachineUser{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, proj)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
	t.Log("user grant lifecycle complete")
}
