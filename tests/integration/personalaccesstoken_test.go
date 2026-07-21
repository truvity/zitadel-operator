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

func TestPersonalAccessToken_WithUserRef(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	userName := fmt.Sprintf("patuser-%d", ts)
	muName := fmt.Sprintf("patmu-%d", ts)
	muSecretName := fmt.Sprintf("patmusecret-%d", ts)
	patName := fmt.Sprintf("pat-%d", ts)
	patSecretName := fmt.Sprintf("patsecret-%d", ts)

	// Create MachineUser CR.
	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      muName,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.MachineUserSpec{
			OrganizationId: testOrgID,
			UserName:       userName,
			KeySecretRef:   zitadelv1alpha2.MachineKeySecretRef{Name: muSecretName},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser CR: %v", err)
	}
	var reconciledMU zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciledMU, 30*time.Second)
	t.Logf("machine user ready: userId=%s", reconciledMU.Status.UserId)

	// Create PersonalAccessToken CR.
	pat := &zitadelv1alpha2.PersonalAccessToken{
		ObjectMeta: metav1.ObjectMeta{
			Name:      patName,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.PersonalAccessTokenSpec{
			OrganizationId: testOrgID,
			UserRef:        &zitadelv1alpha2.ResourceRef{Name: muName},
			TokenSecretRef: zitadelv1alpha2.TokenSecretRefSpec{Name: patSecretName},
		},
	}
	if err := k8sClient.Create(ctx, pat); err != nil {
		t.Fatalf("creating PersonalAccessToken CR: %v", err)
	}

	var reconciledPAT zitadelv1alpha2.PersonalAccessToken
	waitForReady(t, ctx, client.ObjectKeyFromObject(pat), &reconciledPAT, 30*time.Second)

	t.Logf("PersonalAccessToken reconciled: tokenId=%s, userId=%s, ready=%v",
		reconciledPAT.Status.TokenId, reconciledPAT.Status.UserId, reconciledPAT.Status.Ready)

	if reconciledPAT.Status.TokenId == "" {
		t.Fatal("expected tokenId to be set")
	}
	if reconciledPAT.Status.UserId != reconciledMU.Status.UserId {
		t.Fatalf("expected userId=%s, got %s", reconciledMU.Status.UserId, reconciledPAT.Status.UserId)
	}

	// Verify Secret was created with token.
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: patSecretName, Namespace: "default"}, secret); err != nil {
		t.Fatalf("getting Secret: %v", err)
	}
	if len(secret.Data["token"]) == 0 {
		t.Fatal("expected token in Secret")
	}
	t.Logf("Secret contains token (%d bytes)", len(secret.Data["token"]))

	// Cleanup.
	if err := k8sClient.Delete(ctx, pat); err != nil {
		t.Fatalf("deleting PersonalAccessToken CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(pat), &zitadelv1alpha2.PersonalAccessToken{}, 30*time.Second)

	if err := k8sClient.Delete(ctx, mu); err != nil {
		t.Fatalf("deleting MachineUser CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(mu), &zitadelv1alpha2.MachineUser{}, 30*time.Second)
	t.Log("PersonalAccessToken lifecycle complete")
}
