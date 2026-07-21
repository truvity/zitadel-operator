//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

func TestMachineUser_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("machuser-%d", ts)
	userName := fmt.Sprintf("bot-%d", ts)
	secretName := fmt.Sprintf("machkey-%d", ts)

	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.MachineUserSpec{
			OrganizationId: testOrgID,
			UserName:       userName,
			Description:    "integration test machine user",
			KeySecretRef: zitadelv1alpha2.MachineKeySecretRef{
				Name: secretName,
			},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser CR: %v", err)
	}

	var reconciledMU zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciledMU, 30*time.Second)

	t.Logf("machine user reconciled: userId=%s, orgId=%s, ready=%v",
		reconciledMU.Status.UserId, reconciledMU.Status.OrganizationId, reconciledMU.Status.Ready)

	if reconciledMU.Status.UserId == "" {
		t.Fatal("expected userId to be set")
	}
	if !reconciledMU.Status.Ready {
		t.Fatal("expected ready=true")
	}

	// Verify key Secret was created.
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, secret); err != nil {
		t.Fatalf("getting key Secret: %v", err)
	}
	if len(secret.Data["key.json"]) == 0 {
		t.Fatal("expected key.json in Secret")
	}
	t.Logf("key Secret contains %d bytes", len(secret.Data["key.json"]))

	// Cleanup.
	if err := k8sClient.Delete(ctx, mu); err != nil {
		t.Fatalf("deleting MachineUser CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(mu), &zitadelv1alpha2.MachineUser{}, 30*time.Second)
	t.Log("machine user lifecycle test complete")
}

func TestMachineUser_WithOrganizationRef(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("muorg-%d", ts)
	muName := fmt.Sprintf("muref-%d", ts)
	userName := fmt.Sprintf("botref-%d", ts)
	secretName := fmt.Sprintf("murefkey-%d", ts)

	// Create Organization first.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)
	t.Logf("org ready: %s (id: %s)", orgName, reconciledOrg.Status.OrganizationId)

	// Create MachineUser referencing the org.
	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{Name: muName, Namespace: "default"},
		Spec: zitadelv1alpha2.MachineUserSpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			UserName:        userName,
			KeySecretRef:    zitadelv1alpha2.MachineKeySecretRef{Name: secretName},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser: %v", err)
	}

	var reconciledMU zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciledMU, 30*time.Second)

	if reconciledMU.Status.OrganizationId != reconciledOrg.Status.OrganizationId {
		t.Fatalf("expected orgId=%s, got %s", reconciledOrg.Status.OrganizationId, reconciledMU.Status.OrganizationId)
	}
	t.Logf("machine user with orgRef reconciled: userId=%s, orgId=%s", reconciledMU.Status.UserId, reconciledMU.Status.OrganizationId)

	// Cleanup.
	_ = k8sClient.Delete(ctx, mu)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(mu), &zitadelv1alpha2.MachineUser{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
}
