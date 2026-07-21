//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

// TestSSA_ForeignManagerConditionSurvives is the v0.18 SSA status-discipline
// regression test (S-210). The prototype demonstrated that read-modify-write
// Status().Update wipes conditions written by another field manager. With SSA
// per-field-manager writes and conditions as listType=map, a condition applied
// by a *different* field manager must survive this operator's own status
// writes. (A zitadel-operator/* manager name would additionally trigger the
// dual-serving AmbiguousInstance gate — that path has its own scenario — so
// this test uses a neutral manager name to isolate the SSA merge behavior.)
func TestSSA_ForeignManagerConditionSurvives(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("v018-ssa-proj-%d", ts)
	appName := fmt.Sprintf("v018-ssa-app-%d", ts)
	secretName := fmt.Sprintf("v018-ssa-secret-%d", ts)

	// Create the OIDCApp before its project exists: the controller keeps
	// writing Ready=False conditions while the ref is unresolved.
	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			ProjectRef:   &zitadelv1alpha2.ResourceRef{Name: projName},
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://v018-ssa.example.com/cb"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: secretName},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, app) })

	// Give the controller time to write its Ready=False condition.
	time.Sleep(3 * time.Second)

	// Apply a condition as a FOREIGN field manager (a second operator bound to
	// another instance) via SSA on the status subresource.
	foreign := &unstructured.Unstructured{}
	foreign.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "zitadel.truvity.io", Version: "v1alpha2", Kind: "OIDCApp",
	})
	foreign.SetName(appName)
	foreign.SetNamespace("default")
	foreign.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":               "ForeignOperatorCheck",
				"status":             "True",
				"reason":             "SetByOtherOperator",
				"message":            "written by another controller's field manager",
				"lastTransitionTime": time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	if err := k8sClient.Status().Patch(ctx, foreign, client.Apply,
		client.FieldOwner("other-controller.example.com")); err != nil {
		t.Fatalf("applying foreign-manager condition: %v", err)
	}

	// Now satisfy the ref: the controller reconciles and writes Ready=True
	// through its own field manager.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
		Spec:       zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, proj) })

	var reconciled zitadelv1alpha2.OIDCApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciled, 60*time.Second)

	// Ready condition (ours) and the foreign condition must BOTH be present.
	ready := findCondition(reconciled.Status.Conditions, "Ready")
	if ready == nil || ready.Status != metav1.ConditionTrue {
		t.Fatalf("expected Ready=True condition, got %+v", ready)
	}
	foreignCond := findCondition(reconciled.Status.Conditions, "ForeignOperatorCheck")
	if foreignCond == nil {
		t.Fatalf("foreign-manager condition was wiped by the operator's status writes; conditions: %+v",
			reconciled.Status.Conditions)
	}
	if foreignCond.Reason != "SetByOtherOperator" {
		t.Fatalf("foreign condition mutated: %+v", foreignCond)
	}
	t.Logf("SSA discipline holds: Ready (ours) and ForeignOperatorCheck (foreign manager) coexist")
}
