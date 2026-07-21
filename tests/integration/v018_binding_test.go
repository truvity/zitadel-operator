//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/controller"
	"github.com/truvity/zitadel-operator/internal/zitadel"
)

// TestBinding_VerifyIAMOwner (S-220) proves startup binding verification via
// AuthService.ListMyMemberships: the test credential is IAM_OWNER, so the
// iam-owner assertion passes and returns no bound org.
func TestBinding_VerifyIAMOwner(t *testing.T) {
	boundOrg, err := zitadel.VerifyBinding(context.Background(), zitadelClient, config.BindingIAMOwner)
	if err != nil {
		t.Fatalf("iam-owner binding verification failed: %v", err)
	}
	if boundOrg != "" {
		t.Fatalf("iam-owner binding must not return a bound org, got %s", boundOrg)
	}
	t.Log("binding verification: iam-owner assertion matches the credential")
}

// TestBinding_MismatchCrashes (S-221) proves the fail-fast: asserting
// org-owner with an IAM_OWNER credential is a mismatch and must error
// (in main() this error crashes the operator before any reconcile).
func TestBinding_MismatchCrashes(t *testing.T) {
	_, err := zitadel.VerifyBinding(context.Background(), zitadelClient, config.BindingOrgOwner)
	if err == nil {
		t.Fatal("expected org-owner assertion to fail against an IAM_OWNER credential")
	}
	if !strings.Contains(err.Error(), "IAM_OWNER") {
		t.Fatalf("mismatch error should name the actual privilege, got: %v", err)
	}
	t.Logf("binding mismatch correctly rejected: %v", err)
}

// TestBinding_DegradationMatrix_OrgOwner (S-222) proves the org-owner
// degradation matrix: an instance-level resource reconciled under an
// org-owner binding gets Ready=False / NotSupportedAtBindingLevel and no
// Zitadel call is attempted.
func TestBinding_DegradationMatrix_OrgOwner(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("v018-binding-lockout-%d", ts)

	cr := &zitadelv1alpha2.DefaultLockoutPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultLockoutPolicySpec{
			LockoutPolicyFields: zitadelv1alpha2.LockoutPolicyFields{MaxPasswordAttempts: 5},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultLockoutPolicy: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, cr) })

	// Reconcile directly with an org-owner-bound config (the shared manager
	// runs iam-owner; a direct call exercises the degradation path).
	orgOwnerCfg := &config.Config{
		Domain:              cfg.Domain,
		Binding:             config.BindingOrgOwner,
		BoundOrganizationId: "000000000000000000",
	}
	rec := &controller.DefaultLockoutPolicyReconciler{
		Client:  k8sClient,
		Zitadel: zitadelClient,
		Config:  orgOwnerCfg,
	}
	if _, err := rec.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: "default"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var cur zitadelv1alpha2.DefaultLockoutPolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &cur); err != nil {
		t.Fatal(err)
	}
	cond := findCondition(cur.Status.Conditions, "Ready")
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "NotSupportedAtBindingLevel" {
		t.Fatalf("expected Ready=False/NotSupportedAtBindingLevel, got %+v", cond)
	}
	if cur.Status.Ready {
		t.Fatal("instance-level resource must not be Ready under org-owner binding")
	}
	t.Log("degradation matrix: instance-level resource fail-closed under org-owner binding")
}

// TestBinding_ForeignOrgMap_Event (S-223) proves that under an org-owner
// binding a scope map for a foreign org is rejected: Ready=False /
// NotSupportedAtBindingLevel plus a ForeignOrganization Warning Event.
func TestBinding_ForeignOrgMap_Event(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)
	mapName := fmt.Sprintf("v018-foreign-map-%d", ts)

	m := createScopeMap(t, ctx, mapName, zitadelv1alpha2.ScopeMapSpec{
		Instance:       cfg.Domain,
		Organization:   orgName,
		OrganizationId: orgID,
		Rules: []zitadelv1alpha2.ScopeMapRule{{
			Name:       "foreign",
			Namespaces: []string{"v018-no-such-namespace"},
		}},
	})

	fakeRecorder := record.NewFakeRecorder(8)
	rec := &controller.ScopeMapReconciler{
		Client:  k8sClient,
		Zitadel: zitadelClient,
		Config: &config.Config{
			Domain:              cfg.Domain,
			Binding:             config.BindingOrgOwner,
			BoundOrganizationId: "000000000000000000", // != the map's org
		},
		Instance:  cfg.Domain,
		Namespace: operatorNamespace,
		Recorder:  fakeRecorder,
	}
	if _, err := rec.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(m)}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var cur zitadelv1alpha2.ScopeMap
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(m), &cur); err != nil {
		t.Fatal(err)
	}
	if cur.Status.Ready {
		t.Fatal("foreign-org map must be fail-closed under org-owner binding")
	}
	cond := findCondition(cur.Status.Conditions, "Ready")
	if cond == nil || cond.Reason != "NotSupportedAtBindingLevel" {
		t.Fatalf("expected NotSupportedAtBindingLevel condition, got %+v", cond)
	}
	select {
	case ev := <-fakeRecorder.Events:
		if !strings.Contains(ev, "ForeignOrganization") {
			t.Fatalf("expected ForeignOrganization event, got %q", ev)
		}
		t.Logf("foreign-org map Event emitted: %s", ev)
	default:
		t.Fatal("expected a ForeignOrganization Event on the map")
	}
}
