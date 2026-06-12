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

func TestApplicationKey_WithAppRef(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("keyproj-%d", ts)
	appName := fmt.Sprintf("keyapp-%d", ts)
	appSecretName := fmt.Sprintf("keyappsecret-%d", ts)
	keyName := fmt.Sprintf("appkey-%d", ts)
	keySecretName := fmt.Sprintf("appkeysecret-%d", ts)

	// Create Project CR.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projName,
			Namespace: "default",
		},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}
	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)

	// Create APIApp CR (private_key_jwt auth method allows key-based access).
	app := &zitadelv1alpha2.APIApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.APIAppSpec{
			ProjectRef: &zitadelv1alpha2.ResourceRef{Name: projName},
			AuthMethod: "private_key_jwt",
			SecretRef:  zitadelv1alpha2.SecretRefSpec{Name: appSecretName},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating APIApp CR: %v", err)
	}
	var reconciledApp zitadelv1alpha2.APIApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciledApp, 30*time.Second)

	// Create ApplicationKey CR.
	key := &zitadelv1alpha2.ApplicationKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyName,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.ApplicationKeySpec{
			ProjectRef:   &zitadelv1alpha2.ResourceRef{Name: projName},
			AppRef:       &zitadelv1alpha2.ResourceRef{Name: appName},
			KeyType:      "json",
			KeySecretRef: zitadelv1alpha2.MachineKeySecretRef{Name: keySecretName},
		},
	}
	if err := k8sClient.Create(ctx, key); err != nil {
		t.Fatalf("creating ApplicationKey CR: %v", err)
	}

	var reconciledKey zitadelv1alpha2.ApplicationKey
	waitForReady(t, ctx, client.ObjectKeyFromObject(key), &reconciledKey, 30*time.Second)

	t.Logf("ApplicationKey reconciled: keyId=%s, appId=%s, ready=%v",
		reconciledKey.Status.KeyId, reconciledKey.Status.AppId, reconciledKey.Status.Ready)

	if reconciledKey.Status.KeyId == "" {
		t.Fatal("expected keyId to be set")
	}

	// Verify Secret was created with key data.
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: keySecretName, Namespace: "default"}, secret); err != nil {
		t.Fatalf("getting Secret: %v", err)
	}
	if len(secret.Data["key.json"]) == 0 {
		t.Fatal("expected key.json in Secret")
	}
	t.Logf("Secret contains key.json (%d bytes)", len(secret.Data["key.json"]))

	// Cleanup.
	if err := k8sClient.Delete(ctx, key); err != nil {
		t.Fatalf("deleting ApplicationKey CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(key), &zitadelv1alpha2.ApplicationKey{}, 30*time.Second)

	if err := k8sClient.Delete(ctx, app); err != nil {
		t.Fatalf("deleting APIApp CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(app), &zitadelv1alpha2.APIApp{}, 30*time.Second)

	if err := k8sClient.Delete(ctx, proj); err != nil {
		t.Fatalf("deleting Project CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
	t.Log("ApplicationKey lifecycle complete")
}
