//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

// waitForReady polls until the given resource has Status.Ready=true.
func waitForReady(t *testing.T, ctx context.Context, key client.ObjectKey, obj client.Object, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(ctx, key, obj); err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if isReady(obj) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s/%s to become ready", key.Namespace, key.Name)
}

// waitForDeletion polls until the given resource is gone.
func waitForDeletion(t *testing.T, ctx context.Context, key client.ObjectKey, obj client.Object, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(ctx, key, obj); err != nil {
			if errors.IsNotFound(err) {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s/%s to be deleted", key.Namespace, key.Name)
}

// isReady checks Status.Ready for all v1alpha2 types.
func isReady(obj client.Object) bool {
	switch o := obj.(type) {
	case *zitadelv1alpha2.Organization:
		return o.Status.Ready
	case *zitadelv1alpha2.Project:
		return o.Status.Ready
	case *zitadelv1alpha2.OIDCApp:
		return o.Status.Ready
	case *zitadelv1alpha2.MachineUser:
		return o.Status.Ready
	default:
		return false
	}
}
