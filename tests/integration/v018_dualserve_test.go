//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/controller"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
)

// TestDualServe_InstancePin_ForeignInstanceIgnored (S-240) proves the
// dual-serving owner filter: an OIDCApp pinned to a different instance is left
// completely untouched (no finalizer, no status writes, no Zitadel app), and
// flipping the pin to this operator's instance hands the CR over.
func TestDualServe_InstancePin_ForeignInstanceIgnored(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)

	nsName := fmt.Sprintf("v018-pin-%d", ts)
	projectName := fmt.Sprintf("v018-pinproj-%d", ts)
	createNamespace(t, ctx, nsName, nil)

	createScopeMap(t, ctx, "v018-map-pin", zitadelv1alpha2.ScopeMapSpec{
		Instance:       cfg.Domain,
		Organization:   orgName,
		OrganizationId: orgID,
		Rules: []zitadelv1alpha2.ScopeMapRule{{
			Name:       "pin-tenant",
			Namespaces: []string{nsName},
			Project:    projectName,
		}},
	})

	var cleanupProjectID, cleanupUserID, cleanupSecretName string
	t.Cleanup(func() {
		cleanupZitadelProject(orgID, cleanupProjectID)
		cleanupDelegate(t, orgID, cleanupUserID, cleanupSecretName)
	})

	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("v018-pinapp-%d", ts), Namespace: nsName},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			Instance:     "some-other-instance.zitadel.example", // not ours
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://v018.example.com/callback"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: fmt.Sprintf("v018-pinsecret-%d", ts)},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, app) })

	// Give the controller time to (not) act, then verify complete hands-off.
	time.Sleep(5 * time.Second)
	var cur zitadelv1alpha2.OIDCApp
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &cur); err != nil {
		t.Fatal(err)
	}
	if cur.Status.Ready || cur.Status.ApplicationId != "" || len(cur.Status.Conditions) != 0 {
		t.Fatalf("foreign-instance CR must be untouched, got status %+v", cur.Status)
	}
	if len(cur.Finalizers) != 0 {
		t.Fatalf("foreign-instance CR must not get a finalizer, got %v", cur.Finalizers)
	}
	t.Log("foreign-instance pin: CR completely untouched")

	// Flip the pin to this operator's instance: the CR is now ours.
	cur.Spec.Instance = cfg.Domain
	if err := k8sClient.Update(ctx, &cur); err != nil {
		t.Fatalf("updating instance pin: %v", err)
	}
	var reconciled zitadelv1alpha2.OIDCApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciled, 60*time.Second)

	sec := findDelegationSecret(t, ctx, projectName)
	cleanupProjectID, cleanupUserID, cleanupSecretName = reconciled.Status.ProjectId, string(sec.Data["user_id"]), sec.Name
	t.Logf("pin flipped to %s: app reconciled via delegated scope (project %s)", cfg.Domain, reconciled.Status.ProjectId)
}

// TestDualServe_AmbiguousInstance_TwoManagers (S-241) is the two-writer SSA
// smoke: operator A (the shared manager, field manager
// zitadel-operator/<domain-A>) reconciles an unpinned OIDCApp; operator B (a
// second reconciler bound to another domain, distinct field manager) then
// serves the same namespace. B must detect A via managedFields and fail
// closed with AmbiguousInstance — without wiping A's conditions — and A must
// fail closed too once it sees B. Pinning spec.instance to A resolves the
// ambiguity and A resumes.
func TestDualServe_AmbiguousInstance_TwoManagers(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()

	projName := fmt.Sprintf("v018-amb-proj-%d", ts)
	appName := fmt.Sprintf("v018-amb-app-%d", ts)

	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
		Spec:       zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, proj) })

	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			ProjectRef:   &zitadelv1alpha2.ResourceRef{Name: projName},
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://v018-amb.example.com/cb"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: fmt.Sprintf("v018-amb-secret-%d", ts)},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, app) })

	// Operator A (shared manager) reconciles the unpinned CR normally.
	var reconciled zitadelv1alpha2.OIDCApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciled, 60*time.Second)
	origURIs := reconciled.Spec.RedirectUris

	// Operator B: same cluster, different instance domain => different SSA
	// field manager (zitadel-operator/other-instance.example.com). No scope
	// resolver: B is in passthrough. B's Zitadel client is never exercised
	// because the gate fails closed before any external call.
	cfgB := &config.Config{Domain: "other-instance.example.com", Binding: config.BindingIAMOwner, Port: "443"}
	opB := &controller.OIDCAppReconciler{
		Client:  k8sClient,
		Zitadel: zitadelClient,
		Config:  cfgB,
	}
	if _, err := opB.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: appName, Namespace: "default"}}); err != nil {
		t.Fatalf("operator B reconcile: %v", err)
	}

	// B must have marked AmbiguousInstance without wiping A's conditions.
	var cur zitadelv1alpha2.OIDCApp
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &cur); err != nil {
		t.Fatal(err)
	}
	amb := findCondition(cur.Status.Conditions, "InstanceResolved")
	if amb == nil || amb.Status != metav1.ConditionFalse || amb.Reason != "AmbiguousInstance" {
		t.Fatalf("expected InstanceResolved=False/AmbiguousInstance from operator B, got %+v", amb)
	}
	if ready := findCondition(cur.Status.Conditions, "Ready"); ready == nil {
		t.Fatal("operator A's Ready condition was wiped by operator B's SSA write")
	}

	// Trigger operator A with a spec change: A now sees B's field manager and
	// must fail closed too — the external app must NOT be updated.
	cur.Spec.RedirectUris = []string{"https://v018-amb-updated.example.com/cb"}
	if err := k8sClient.Update(ctx, &cur); err != nil {
		t.Fatalf("updating spec: %v", err)
	}
	time.Sleep(5 * time.Second)

	getResp, err := zitadelClient.Application().GetApplication(ctx, &applicationv2.GetApplicationRequest{
		ApplicationId: reconciled.Status.ApplicationId,
	})
	if err != nil {
		t.Fatalf("reading app from Zitadel: %v", err)
	}
	gotURIs := getResp.GetApplication().GetOidcConfiguration().GetRedirectUris()
	if len(gotURIs) != len(origURIs) || gotURIs[0] != origURIs[0] {
		t.Fatalf("ambiguous CR was reconciled externally: URIs %v (want unchanged %v)", gotURIs, origURIs)
	}
	t.Log("both operators fail-closed on the ambiguous CR; no external action")

	// Pin to operator A: ambiguity resolves and A applies the pending change.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &cur); err != nil {
		t.Fatal(err)
	}
	cur.Spec.Instance = cfg.Domain
	if err := k8sClient.Update(ctx, &cur); err != nil {
		t.Fatalf("pinning instance: %v", err)
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		getResp, err = zitadelClient.Application().GetApplication(ctx, &applicationv2.GetApplicationRequest{
			ApplicationId: reconciled.Status.ApplicationId,
		})
		if err == nil {
			uris := getResp.GetApplication().GetOidcConfiguration().GetRedirectUris()
			if len(uris) == 1 && uris[0] == "https://v018-amb-updated.example.com/cb" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for pinned operator to apply the pending change")
		}
		time.Sleep(1 * time.Second)
	}
	t.Log("pin to this operator resolved the ambiguity; pending spec change applied")
}
