//go:build integration

// v0.18 scope-map tests (INF-423/INF-425/INF-435).
//
// All Zitadel resources created here are prefixed v018- and live in the
// "Truvity Testing" org when present (fallback: the binding credential's org).
package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/delegation"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/auth"
	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	objectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object/v2"
	orgv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/org/v2"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
	userv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user"
	userv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user/v2"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const testOrgName = "Truvity Testing"

// testOrg returns (orgID, orgName) for scope-map tests: the "Truvity Testing"
// org when it exists, otherwise the config default org with its actual name.
func testOrg(t *testing.T, ctx context.Context) (string, string) {
	t.Helper()
	resp, err := zitadelClient.Organization().ListOrganizations(ctx, &orgv2.ListOrganizationsRequest{
		Queries: []*orgv2.SearchQuery{{
			Query: &orgv2.SearchQuery_NameQuery{
				NameQuery: &orgv2.OrganizationNameQuery{
					Name:   testOrgName,
					Method: objectv2.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
				},
			},
		}},
	})
	if err == nil {
		for _, o := range resp.GetResult() {
			if o.GetName() == testOrgName {
				return o.GetId(), o.GetName()
			}
		}
	}
	// Fallback: the binding credential's own org.
	resp, err = zitadelClient.Organization().ListOrganizations(ctx, &orgv2.ListOrganizationsRequest{
		Queries: []*orgv2.SearchQuery{{
			Query: &orgv2.SearchQuery_IdQuery{
				IdQuery: &orgv2.OrganizationIDQuery{Id: testOrgID},
			},
		}},
	})
	if err != nil || len(resp.GetResult()) == 0 {
		t.Fatalf("resolving test org: %v", err)
	}
	return resp.GetResult()[0].GetId(), resp.GetResult()[0].GetName()
}

// createNamespace creates a tenant namespace (never deleted: envtest has no
// namespace controller; unique names avoid clashes).
func createNamespace(t *testing.T, ctx context.Context, name string, labels map[string]string) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
	if err := k8sClient.Create(ctx, ns); err != nil {
		t.Fatalf("creating namespace %s: %v", name, err)
	}
}

// createScopeMap creates a map in the operator namespace and registers cleanup.
func createScopeMap(t *testing.T, ctx context.Context, name string, spec zitadelv1alpha2.ZitadelScopeMapSpec) *zitadelv1alpha2.ZitadelScopeMap {
	t.Helper()
	m := &zitadelv1alpha2.ZitadelScopeMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: operatorNamespace},
		Spec:       spec,
	}
	if err := k8sClient.Create(ctx, m); err != nil {
		t.Fatalf("creating ZitadelScopeMap %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), m)
		waitForDeletion(t, context.Background(), client.ObjectKeyFromObject(m), &zitadelv1alpha2.ZitadelScopeMap{}, 30*time.Second)
	})
	return m
}

// waitForOIDCAppCondition polls until the OIDCApp has the given condition
// status+reason.
func waitForOIDCAppCondition(t *testing.T, ctx context.Context, key client.ObjectKey, condType string, status metav1.ConditionStatus, reason string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		var app zitadelv1alpha2.OIDCApp
		if err := k8sClient.Get(ctx, key, &app); err == nil {
			for _, c := range app.Status.Conditions {
				if c.Type == condType {
					last = fmt.Sprintf("%s/%s: %s", c.Status, c.Reason, c.Message)
					if c.Status == status && c.Reason == reason {
						return
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for OIDCApp %s condition %s=%s reason=%s (last: %s)", key.Name, condType, status, reason, last)
}

// cleanupZitadelProject best-effort deletes a prototype project.
func cleanupZitadelProject(orgID, projectID string) {
	if projectID == "" {
		return
	}
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-zitadel-orgid", orgID)
	_, _ = zitadelClient.Project().DeleteProject(ctx, &projectv2.DeleteProjectRequest{ProjectId: projectID})
}

// cleanupDelegate best-effort deletes the delegate machine user and Secret
// for the given scope project so reruns start clean.
func cleanupDelegate(t *testing.T, orgID, userID, secretName string) {
	t.Helper()
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-zitadel-orgid", orgID)
	if userID != "" {
		_, _ = zitadelClient.Management().RemoveUser(ctx, &management.RemoveUserRequest{Id: userID}) //nolint:staticcheck // SA1019
	}
	if secretName != "" {
		_ = k8sClient.Delete(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: operatorNamespace},
		})
	}
}

// findDelegationSecret returns the delegation Secret whose scope.json
// references the given project name.
func findDelegationSecret(t *testing.T, ctx context.Context, projectName string) *corev1.Secret {
	t.Helper()
	var secrets corev1.SecretList
	if err := k8sClient.List(ctx, &secrets,
		client.InNamespace(operatorNamespace),
		client.MatchingLabels{delegation.DelegationLabel: "true"},
	); err != nil {
		t.Fatalf("listing delegation secrets: %v", err)
	}
	for i := range secrets.Items {
		s := &secrets.Items[i]
		if strings.Contains(string(s.Data["scope.json"]), projectName) {
			return s
		}
	}
	t.Fatalf("no delegation Secret found for project %q (found %d delegation secrets)", projectName, len(secrets.Items))
	return nil
}

// --- Goal 1: selector rule match --------------------------------------------

func TestScopeMap_SelectorRuleMatch(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)

	nsName := fmt.Sprintf("v018-sel-%d", ts)
	projectName := fmt.Sprintf("v018-selproj-%d", ts)
	createNamespace(t, ctx, nsName, map[string]string{"v018-scope": "alpha"})

	createScopeMap(t, ctx, "v018-map-sel", zitadelv1alpha2.ZitadelScopeMapSpec{
		Instance:       cfg.Domain,
		Organization:   orgName,
		OrganizationId: orgID,
		Rules: []zitadelv1alpha2.ScopeMapRule{{
			Name: "alpha-team",
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"v018-scope": "alpha"},
			},
			Project: projectName,
		}},
	})

	// Zitadel-side cleanup: registered BEFORE the app cleanup so it runs
	// AFTER it (LIFO) — the delegate must survive until the app is deleted.
	var cleanupProjectID, cleanupUserID, cleanupSecretName string
	t.Cleanup(func() {
		cleanupZitadelProject(orgID, cleanupProjectID)
		cleanupDelegate(t, orgID, cleanupUserID, cleanupSecretName)
	})

	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("v018-selapp-%d", ts), Namespace: nsName},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			// No projectRef/projectId: defaults to the scope's project.
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://v018.example.com/callback"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: fmt.Sprintf("v018-selsecret-%d", ts)},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), app)
		waitForDeletion(t, context.Background(), client.ObjectKeyFromObject(app), &zitadelv1alpha2.OIDCApp{}, 30*time.Second)
	})

	var reconciled zitadelv1alpha2.OIDCApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciled, 60*time.Second)

	if reconciled.Status.ProjectId == "" {
		t.Fatal("expected scope-defaulted projectId in status")
	}
	if reconciled.Status.OrganizationId != orgID {
		t.Fatalf("expected orgId %s, got %s", orgID, reconciled.Status.OrganizationId)
	}
	waitForOIDCAppCondition(t, ctx, client.ObjectKeyFromObject(app), "ScopeResolved", metav1.ConditionTrue, "Resolved", 10*time.Second)

	// Verify the scope project exists in Zitadel with the rule's name.
	sec := findDelegationSecret(t, ctx, projectName)
	delegateUserID := string(sec.Data["user_id"])
	cleanupProjectID, cleanupUserID, cleanupSecretName = reconciled.Status.ProjectId, delegateUserID, sec.Name
	if got := string(sec.Data["project_id"]); got != reconciled.Status.ProjectId {
		t.Fatalf("delegation secret project_id %s != status projectId %s", got, reconciled.Status.ProjectId)
	}
	t.Logf("selector rule matched: ns=%s project=%s (%s) delegate=%s",
		nsName, projectName, reconciled.Status.ProjectId, delegateUserID)
}

// --- Goal 1 + Goal 2: literal rule match + delegated ACTOR PROOF ------------

func TestScopeMap_LiteralMatch_DelegatedActorProof(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)

	nsName := fmt.Sprintf("v018-lit-%d", ts)
	projectName := fmt.Sprintf("v018-litproj-%d", ts)
	createNamespace(t, ctx, nsName, nil)

	createScopeMap(t, ctx, "v018-map-lit", zitadelv1alpha2.ZitadelScopeMapSpec{
		Instance:       cfg.Domain,
		Organization:   orgName,
		OrganizationId: orgID,
		Rules: []zitadelv1alpha2.ScopeMapRule{{
			Name:       "literal-tenant",
			Namespaces: []string{nsName},
			Project:    projectName,
		}},
	})

	// Zitadel-side cleanup: registered BEFORE the app cleanup so it runs
	// AFTER it (LIFO) — the delegate must survive until the app is deleted.
	var cleanupProjectID, cleanupUserID, cleanupSecretName string
	t.Cleanup(func() {
		cleanupZitadelProject(orgID, cleanupProjectID)
		cleanupDelegate(t, orgID, cleanupUserID, cleanupSecretName)
	})

	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("v018-litapp-%d", ts), Namespace: nsName},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://v018.example.com/callback"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: fmt.Sprintf("v018-litsecret-%d", ts)},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), app)
		waitForDeletion(t, context.Background(), client.ObjectKeyFromObject(app), &zitadelv1alpha2.OIDCApp{}, 30*time.Second)
	})

	var reconciled zitadelv1alpha2.OIDCApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciled, 60*time.Second)
	projectID := reconciled.Status.ProjectId
	if projectID == "" {
		t.Fatal("expected scope-defaulted projectId in status")
	}

	// The delegate identity, from the cached delegation Secret.
	sec := findDelegationSecret(t, ctx, projectName)
	delegateUserID := string(sec.Data["user_id"])
	cleanupProjectID, cleanupUserID, cleanupSecretName = projectID, delegateUserID, sec.Name
	if delegateUserID == "" {
		t.Fatal("delegation secret has no user_id")
	}
	if bindingUserID == "" {
		t.Fatal("bindingUserID not initialized")
	}
	if delegateUserID == bindingUserID {
		t.Fatalf("delegate user %s must differ from binding user %s", delegateUserID, bindingUserID)
	}

	// ACTOR PROOF: the application-creation events on the project aggregate
	// must be authored by the delegated SA, not the binding credential.
	evCtx := metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)
	evResp, err := zitadelClient.Admin().ListEvents(evCtx, &admin.ListEventsRequest{
		AggregateId:    projectID,
		AggregateTypes: []string{"project"},
		Limit:          200,
		Asc:            true,
	})
	if err != nil {
		t.Fatalf("listing events for project %s: %v", projectID, err)
	}

	var appEvents, delegateAppEvents, bindingAppEvents int
	var projectAddedEditor string
	for _, ev := range evResp.GetEvents() {
		evType := ev.GetType().GetType()
		editor := ev.GetEditor().GetUserId()
		if evType == "project.added" {
			projectAddedEditor = editor
		}
		if strings.Contains(evType, "application") {
			appEvents++
			switch editor {
			case delegateUserID:
				delegateAppEvents++
			case bindingUserID:
				bindingAppEvents++
			}
			t.Logf("event %s editor=%s (delegate=%v)", evType, editor, editor == delegateUserID)
		}
	}
	if appEvents == 0 {
		t.Fatal("no application events found on project aggregate")
	}
	if bindingAppEvents != 0 {
		t.Fatalf("SECURITY: %d application events authored by the binding credential", bindingAppEvents)
	}
	if delegateAppEvents != appEvents {
		t.Fatalf("expected all %d application events by delegate %s, got %d", appEvents, delegateUserID, delegateAppEvents)
	}
	// The project itself is created by the binding credential (delegation
	// minting), which is expected and documents the trust boundary.
	t.Logf("ACTOR PROOF: %d application events all authored by delegate %s (binding %s authored project.added: %v)",
		appEvents, delegateUserID, bindingUserID, projectAddedEditor == bindingUserID)
}

// --- Goal 1: no-match fail-closed -------------------------------------------

func TestScopeMap_NoMatch_FailClosed(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)

	nsName := fmt.Sprintf("v018-nomatch-%d", ts)
	createNamespace(t, ctx, nsName, nil) // no labels, not listed anywhere

	createScopeMap(t, ctx, "v018-map-nomatch", zitadelv1alpha2.ZitadelScopeMapSpec{
		Instance:       cfg.Domain,
		Organization:   orgName,
		OrganizationId: orgID,
		Rules: []zitadelv1alpha2.ScopeMapRule{{
			Name:       "other-tenant",
			Namespaces: []string{"some-other-namespace"},
		}},
	})

	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("v018-nomatchapp-%d", ts), Namespace: nsName},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			ProjectId:    "000000000000000000", // never reached: fail-closed first
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://v018.example.com/callback"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: "v018-nomatchsecret"},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), app)
		waitForDeletion(t, context.Background(), client.ObjectKeyFromObject(app), &zitadelv1alpha2.OIDCApp{}, 30*time.Second)
	})

	waitForOIDCAppCondition(t, ctx, client.ObjectKeyFromObject(app), "ScopeResolved", metav1.ConditionFalse, "NoMatchingRule", 30*time.Second)

	var cur zitadelv1alpha2.OIDCApp
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &cur); err != nil {
		t.Fatal(err)
	}
	if cur.Status.Ready {
		t.Fatal("fail-closed namespace must not become ready")
	}
	if cur.Status.ApplicationId != "" {
		t.Fatal("fail-closed namespace must not create a Zitadel application")
	}
	t.Log("no-match fail-closed verified: ScopeResolved=False/NoMatchingRule, no app created")
}

// --- Goal 1: cross-map conflict ---------------------------------------------

func TestScopeMap_CrossMapConflict(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)

	nsName := fmt.Sprintf("v018-conflict-%d", ts)
	createNamespace(t, ctx, nsName, nil)

	spec := func(rule string) zitadelv1alpha2.ZitadelScopeMapSpec {
		return zitadelv1alpha2.ZitadelScopeMapSpec{
			Instance:       cfg.Domain,
			Organization:   orgName,
			OrganizationId: orgID,
			Rules: []zitadelv1alpha2.ScopeMapRule{{
				Name:       rule,
				Namespaces: []string{nsName},
			}},
		}
	}
	m1 := createScopeMap(t, ctx, "v018-map-conflict-a", spec("rule-a"))
	m2 := createScopeMap(t, ctx, "v018-map-conflict-b", spec("rule-b"))

	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("v018-conflictapp-%d", ts), Namespace: nsName},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			ProjectId:    "000000000000000000",
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://v018.example.com/callback"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: "v018-conflictsecret"},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), app)
		waitForDeletion(t, context.Background(), client.ObjectKeyFromObject(app), &zitadelv1alpha2.OIDCApp{}, 30*time.Second)
	})

	waitForOIDCAppCondition(t, ctx, client.ObjectKeyFromObject(app), "ScopeResolved", metav1.ConditionFalse, "ScopeConflict", 30*time.Second)

	// Events must exist on BOTH maps.
	deadline := time.Now().Add(15 * time.Second)
	for {
		found := map[string]bool{}
		var events corev1.EventList
		if err := k8sClient.List(ctx, &events, client.InNamespace(operatorNamespace)); err == nil {
			for _, ev := range events.Items {
				if ev.Reason == "ScopeConflict" &&
					(ev.InvolvedObject.Name == m1.Name || ev.InvolvedObject.Name == m2.Name) {
					found[ev.InvolvedObject.Name] = true
				}
			}
		}
		if found[m1.Name] && found[m2.Name] {
			t.Logf("conflict Events present on both maps: %v", found)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for ScopeConflict events on both maps, got %v", found)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// --- Goal 1: instance mismatch ----------------------------------------------

func TestScopeMap_InstanceMismatch(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)

	nsName := fmt.Sprintf("v018-mismatch-%d", ts)
	createNamespace(t, ctx, nsName, nil)

	m := createScopeMap(t, ctx, "v018-map-mismatch", zitadelv1alpha2.ZitadelScopeMapSpec{
		Instance:       "wrong-instance.zitadel.example", // != operator binding
		Organization:   orgName,
		OrganizationId: orgID,
		Rules: []zitadelv1alpha2.ScopeMapRule{{
			Name:       "mismatched",
			Namespaces: []string{nsName},
		}},
	})

	// The map controller must mark the map fail-closed.
	deadline := time.Now().Add(30 * time.Second)
	for {
		var cur zitadelv1alpha2.ZitadelScopeMap
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(m), &cur); err == nil {
			var matched bool
			for _, c := range cur.Status.Conditions {
				if c.Type == "InstanceMatch" && c.Status == metav1.ConditionFalse && c.Reason == "InstanceMismatch" {
					matched = true
				}
			}
			if matched && !cur.Status.Ready {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for InstanceMatch=False on mismatched map")
		}
		time.Sleep(500 * time.Millisecond)
	}

	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("v018-mismatchapp-%d", ts), Namespace: nsName},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			ProjectId:    "000000000000000000",
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://v018.example.com/callback"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: "v018-mismatchsecret"},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), app)
		waitForDeletion(t, context.Background(), client.ObjectKeyFromObject(app), &zitadelv1alpha2.OIDCApp{}, 30*time.Second)
	})

	waitForOIDCAppCondition(t, ctx, client.ObjectKeyFromObject(app), "ScopeResolved", metav1.ConditionFalse, "InstanceMismatch", 30*time.Second)

	var cur zitadelv1alpha2.OIDCApp
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &cur); err != nil {
		t.Fatal(err)
	}
	if cur.Status.Ready || cur.Status.ApplicationId != "" {
		t.Fatal("instance-mismatched namespace must stay fail-closed")
	}
	t.Log("instance mismatch fail-closed verified on both the map and the OIDCApp")
}

// --- Goal 3: SDK surface evidence -------------------------------------------

// TestScopeMap_SDKSurface_OwnMemberships proves zitadel-go v3.29.2 exposes
// AuthService.ListMyMemberships: the operator can verify its own binding
// (iam-owner vs org-owner) at startup.
func TestScopeMap_SDKSurface_OwnMemberships(t *testing.T) {
	ctx := context.Background()
	resp, err := zitadelClient.Auth().ListMyMemberships(ctx, &auth.ListMyMembershipsRequest{})
	if err != nil {
		t.Fatalf("ListMyMemberships: %v", err)
	}
	if len(resp.GetResult()) == 0 {
		t.Fatal("binding credential has no memberships (expected at least IAM_OWNER or ORG_OWNER)")
	}
	for _, m := range resp.GetResult() {
		t.Logf("binding membership: roles=%v iam=%v orgId=%q projectId=%q",
			m.GetRoles(), m.GetIam(), m.GetOrgId(), m.GetProjectId())
	}
}

// TestScopeMap_SDKSurface_DualKeyRotation proves the v2 UserService supports
// the dual-key rotation flow: AddKey / ListKeys / RemoveKey on a machine user
// with two keys live at once.
func TestScopeMap_SDKSurface_DualKeyRotation(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, _ := testOrg(t, ctx)
	orgCtx := metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Create a scratch machine user.
	createResp, err := zitadelClient.Management().AddMachineUser(orgCtx, &management.AddMachineUserRequest{ //nolint:staticcheck // SA1019
		UserName:        fmt.Sprintf("v018-rotate-%d", ts),
		Name:            "v018 dual-key rotation probe",
		AccessTokenType: userv1.AccessTokenType_ACCESS_TOKEN_TYPE_BEARER,
	})
	if err != nil {
		t.Fatalf("AddMachineUser: %v", err)
	}
	userID := createResp.GetUserId()
	t.Cleanup(func() {
		_, _ = zitadelClient.Management().RemoveUser(orgCtx, &management.RemoveUserRequest{Id: userID}) //nolint:staticcheck // SA1019
	})

	addKey := func() string {
		resp, err := zitadelClient.User().AddKey(orgCtx, &userv2.AddKeyRequest{
			UserId:         userID,
			ExpirationDate: timestamppb.New(time.Now().Add(24 * time.Hour)),
		})
		if err != nil {
			t.Fatalf("AddKey: %v", err)
		}
		if len(resp.GetKeyContent()) == 0 {
			t.Fatal("AddKey returned no key content")
		}
		return resp.GetKeyId()
	}
	key1, key2 := addKey(), addKey()

	listKeys := func() int {
		resp, err := zitadelClient.User().ListKeys(orgCtx, &userv2.ListKeysRequest{
			Filters: []*userv2.KeysSearchFilter{{
				Filter: &userv2.KeysSearchFilter_UserIdFilter{
					UserIdFilter: &filterv2.IDFilter{Id: userID},
				},
			}},
		})
		if err != nil {
			t.Fatalf("ListKeys: %v", err)
		}
		return len(resp.GetResult())
	}
	if got := listKeys(); got != 2 {
		t.Fatalf("expected 2 live keys during rotation, got %d", got)
	}

	// Retire the old key.
	if _, err := zitadelClient.User().RemoveKey(orgCtx, &userv2.RemoveKeyRequest{UserId: userID, KeyId: key1}); err != nil {
		t.Fatalf("RemoveKey: %v", err)
	}
	if got := listKeys(); got != 1 {
		t.Fatalf("expected 1 key after retiring old key, got %d", got)
	}
	t.Logf("dual-key rotation verified: add %s + %s -> remove %s -> 1 key live", key1, key2, key1)
}
