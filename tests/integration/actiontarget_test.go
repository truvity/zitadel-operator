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

func TestActionTarget_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("target-%d", time.Now().UnixMilli())

	target := &zitadelv1alpha2.ActionTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.ActionTargetSpec{
			Endpoint:         "https://httpbin.org/post",
			Timeout:          "10s",
			InterruptOnError: true,
		},
	}
	if err := k8sClient.Create(ctx, target); err != nil {
		t.Fatalf("creating ActionTarget: %v", err)
	}

	var reconciledTarget zitadelv1alpha2.ActionTarget
	waitForReady(t, ctx, client.ObjectKeyFromObject(target), &reconciledTarget, 30*time.Second)

	t.Logf("action target reconciled: targetId=%s, ready=%v", reconciledTarget.Status.TargetId, reconciledTarget.Status.Ready)

	if reconciledTarget.Status.TargetId == "" {
		t.Fatal("expected targetId to be set")
	}

	// Cleanup.
	if err := k8sClient.Delete(ctx, target); err != nil {
		t.Fatalf("deleting ActionTarget: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(target), &zitadelv1alpha2.ActionTarget{}, 30*time.Second)
	t.Log("action target lifecycle complete")
}

func TestActionExecution_WithTargetRef(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	targetName := fmt.Sprintf("exectarget-%d", ts)
	execName := fmt.Sprintf("exec-%d", ts)

	// Create ActionTarget first.
	target := &zitadelv1alpha2.ActionTarget{
		ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: "default"},
		Spec: zitadelv1alpha2.ActionTargetSpec{
			Endpoint: "https://httpbin.org/post",
			Timeout:  "10s",
		},
	}
	if err := k8sClient.Create(ctx, target); err != nil {
		t.Fatalf("creating ActionTarget: %v", err)
	}
	var reconciledTarget zitadelv1alpha2.ActionTarget
	waitForReady(t, ctx, client.ObjectKeyFromObject(target), &reconciledTarget, 30*time.Second)
	t.Logf("target ready: %s (id: %s)", targetName, reconciledTarget.Status.TargetId)

	// Create ActionExecution referencing the target.
	exec := &zitadelv1alpha2.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: execName, Namespace: "default"},
		Spec: zitadelv1alpha2.ActionExecutionSpec{
			Condition: zitadelv1alpha2.ActionCondition{
				Request: "/zitadel.session.v2.SessionService/SetSession",
			},
			Targets: []zitadelv1alpha2.ActionExecutionTarget{
				{TargetRef: &zitadelv1alpha2.ResourceRef{Name: targetName}},
			},
		},
	}
	if err := k8sClient.Create(ctx, exec); err != nil {
		t.Fatalf("creating ActionExecution: %v", err)
	}

	var reconciledExec zitadelv1alpha2.ActionExecution
	waitForReady(t, ctx, client.ObjectKeyFromObject(exec), &reconciledExec, 30*time.Second)

	t.Logf("action execution reconciled: ready=%v", reconciledExec.Status.Ready)

	// Cleanup (execution first, then target).
	_ = k8sClient.Delete(ctx, exec)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(exec), &zitadelv1alpha2.ActionExecution{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, target)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(target), &zitadelv1alpha2.ActionTarget{}, 30*time.Second)
	t.Log("action execution with targetRef lifecycle complete")
}
