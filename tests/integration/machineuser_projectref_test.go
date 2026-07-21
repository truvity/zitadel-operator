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

// TestMachineUser_RolesWithProjectRef covers the v0.19 fleet shape: no scope
// maps, the grant target is an in-namespace Project CR named via
// spec.projectRef.
func TestMachineUser_RolesWithProjectRef(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("murole-proj-%d", ts)
	roleName := fmt.Sprintf("murole-role-%d", ts)
	muName := fmt.Sprintf("murole-mu-%d", ts)
	userName := fmt.Sprintf("murole-bot-%d", ts)
	secretName := fmt.Sprintf("murole-key-%d", ts)
	roleKey := fmt.Sprintf("default:writer-%d", ts)

	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
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
	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)

	// The role the machine user will be granted.
	role := &zitadelv1alpha2.ProjectRole{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: "default"},
		Spec: zitadelv1alpha2.ProjectRoleSpec{
			ProjectRef: &zitadelv1alpha2.ResourceRef{Name: projName},
			Key:        roleKey,
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("creating ProjectRole CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, role)
		}
	})
	waitForReady(t, ctx, client.ObjectKeyFromObject(role), &zitadelv1alpha2.ProjectRole{}, 30*time.Second)

	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{Name: muName, Namespace: "default"},
		Spec: zitadelv1alpha2.MachineUserSpec{
			OrganizationId: testOrgID,
			UserName:       userName,
			Description:    "v0.19 projectRef grant test",
			ProjectRef:     &zitadelv1alpha2.ResourceRef{Name: projName},
			Roles:          []string{roleKey},
			KeySecretRef: zitadelv1alpha2.MachineKeySecretRef{
				Name: secretName,
			},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, mu)
		}
	})

	var reconciledMU zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciledMU, 60*time.Second)

	if reconciledMU.Status.ProjectId != reconciledProj.Status.ProjectId {
		t.Fatalf("expected grant on project %s, got %s", reconciledProj.Status.ProjectId, reconciledMU.Status.ProjectId)
	}
	if reconciledMU.Status.GrantId == "" {
		t.Fatal("expected a user grant to be recorded in status")
	}
	t.Logf("machine user granted %s on project %s (grant %s)",
		roleKey, reconciledMU.Status.ProjectId, reconciledMU.Status.GrantId)
}
