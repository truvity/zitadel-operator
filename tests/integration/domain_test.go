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

func TestDomain_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("dom-%d", ts)

	// Create Domain CR using default organization from config.
	dom := &zitadelv1alpha2.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DomainSpec{
			OrganizationId: testOrgID,
			DomainName:     fmt.Sprintf("test-%d.example.org", ts),
		},
	}
	if err := k8sClient.Create(ctx, dom); err != nil {
		t.Fatalf("creating Domain: %v", err)
	}

	var reconciled zitadelv1alpha2.Domain
	waitForReady(t, ctx, client.ObjectKeyFromObject(dom), &reconciled, 30*time.Second)

	t.Logf("domain reconciled: domain=%s, ready=%v", reconciled.Spec.DomainName, reconciled.Status.Ready)

	if !reconciled.Status.Ready {
		t.Fatal("expected ready=true")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, dom)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(dom), &zitadelv1alpha2.Domain{}, 30*time.Second)
	t.Log("domain lifecycle complete")
}
