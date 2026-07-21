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

func TestOrgMetadata_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("meta-%d", ts)

	// Create OrgMetadata CR using default organization from config.
	meta := &zitadelv1alpha2.OrgMetadata{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.OrgMetadataSpec{
			OrganizationId: testOrgID,
			Key:            fmt.Sprintf("test-key-%d", ts),
			Value:          "test-value",
		},
	}
	if err := k8sClient.Create(ctx, meta); err != nil {
		t.Fatalf("creating OrgMetadata: %v", err)
	}

	var reconciled zitadelv1alpha2.OrgMetadata
	waitForReady(t, ctx, client.ObjectKeyFromObject(meta), &reconciled, 30*time.Second)

	t.Logf("orgmetadata reconciled: key=%s, ready=%v", reconciled.Spec.Key, reconciled.Status.Ready)

	if !reconciled.Status.Ready {
		t.Fatal("expected ready=true")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, meta)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(meta), &zitadelv1alpha2.OrgMetadata{}, 30*time.Second)
	t.Log("orgmetadata lifecycle complete")
}
