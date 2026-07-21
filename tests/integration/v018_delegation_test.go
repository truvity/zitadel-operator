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
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// newTestDelegationManager builds a fresh Manager (empty cache) sharing the
// harness clients — a fresh instance models an operator restart.
func newTestDelegationManager() *delegation.Manager {
	return &delegation.Manager{
		K8s:     k8sClient,
		Binding: zitadelClient,
		ClientCfg: zitadel.ClientConfig{
			Domain:            cfg.Domain,
			Port:              cfg.Port,
			InsecurePlaintext: cfg.Insecure,
		},
		Namespace:      operatorNamespace,
		UsernamePrefix: "v018-delegate-",
	}
}

func testScope(orgID, orgName, project string) *scopemap.Scope {
	return &scopemap.Scope{
		Instance:         cfg.Domain,
		OrganizationID:   orgID,
		OrganizationName: orgName,
		ProjectName:      project,
		MapName:          "v018-direct",
		RuleName:         "direct",
	}
}

func delegateGone(ctx context.Context, orgID, userID string) bool {
	orgCtx := metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)
	_, err := zitadelClient.Management().GetUserByID(orgCtx, &management.GetUserByIDRequest{Id: userID}) //nolint:staticcheck // SA1019
	return status.Code(err) == codes.NotFound
}

// TestDelegation_WarmRestart (S-231): a delegate minted before a restart is
// reused from its Secret by a fresh Manager — no second SA is minted.
func TestDelegation_WarmRestart(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)
	scope := testScope(orgID, orgName, fmt.Sprintf("v018-warm-%d", ts))

	m1 := newTestDelegationManager()
	d1, err := m1.Ensure(ctx, scope)
	if err != nil {
		t.Fatalf("initial mint: %v", err)
	}
	t.Cleanup(func() {
		_ = m1.Revoke(context.Background(), scope.Hash())
		cleanupZitadelProject(orgID, d1.ProjectID)
	})

	// "Restart": fresh manager, warm from Secrets, ensure again.
	m2 := newTestDelegationManager()
	if err := m2.WarmFromSecrets(ctx); err != nil {
		t.Fatalf("warm from secrets: %v", err)
	}
	d2, err := m2.Ensure(ctx, scope)
	if err != nil {
		t.Fatalf("post-restart ensure: %v", err)
	}
	if d2.UserID != d1.UserID {
		t.Fatalf("warm restart minted a second delegate: %s != %s", d2.UserID, d1.UserID)
	}
	if d2.KeyID != d1.KeyID {
		t.Fatalf("warm restart replaced the key: %s != %s", d2.KeyID, d1.KeyID)
	}
	t.Logf("warm restart reused delegate %s (key %s) from Secret", d2.UserID, d2.KeyID)
}

// TestDelegation_LazyRemint (S-232): when the delegate SA is deleted
// out-of-band, the next Ensure detects the stale credential and re-mints.
func TestDelegation_LazyRemint(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)
	scope := testScope(orgID, orgName, fmt.Sprintf("v018-remint-%d", ts))
	orgCtx := metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	m1 := newTestDelegationManager()
	d1, err := m1.Ensure(ctx, scope)
	if err != nil {
		t.Fatalf("initial mint: %v", err)
	}
	t.Cleanup(func() {
		m2 := newTestDelegationManager()
		_ = m2.Revoke(context.Background(), scope.Hash())
		cleanupZitadelProject(orgID, d1.ProjectID)
	})

	// Out-of-band SA deletion.
	if _, err := zitadelClient.Management().RemoveUser(orgCtx, &management.RemoveUserRequest{Id: d1.UserID}); err != nil { //nolint:staticcheck // SA1019
		t.Fatalf("out-of-band delete: %v", err)
	}

	// A fresh manager (restart) warms the stale Secret, validates lazily,
	// drops it and re-mints.
	m2 := newTestDelegationManager()
	d2, err := m2.Ensure(ctx, scope)
	if err != nil {
		t.Fatalf("re-mint ensure: %v", err)
	}
	if d2.UserID == d1.UserID {
		t.Fatalf("expected a re-minted delegate, got the deleted one back: %s", d2.UserID)
	}

	// The Secret must now carry the new identity.
	var sec corev1.Secret
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: delegation.SecretNamePrefix + scope.Hash(), Namespace: operatorNamespace}, &sec); err != nil {
		t.Fatalf("reading re-minted Secret: %v", err)
	}
	if string(sec.Data["user_id"]) != d2.UserID {
		t.Fatalf("Secret user_id %s != re-minted delegate %s", sec.Data["user_id"], d2.UserID)
	}
	t.Logf("lazy re-mint verified: %s (deleted out-of-band) -> %s", d1.UserID, d2.UserID)
}

// TestDelegation_Rotation_DualKey (S-233): delegate keys past the rotation
// cycle are replaced with a fresh key; the old key stays valid during the
// grace overlap and is revoked by the sweep afterwards.
func TestDelegation_Rotation_DualKey(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)
	scope := testScope(orgID, orgName, fmt.Sprintf("v018-rot-%d", ts))
	orgCtx := metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	m := newTestDelegationManager()
	m.RotateAfter = 1 * time.Millisecond // everything is instantly due
	m.RotationGrace = 2 * time.Second

	d1, err := m.Ensure(ctx, scope)
	if err != nil {
		t.Fatalf("initial mint: %v", err)
	}
	t.Cleanup(func() {
		_ = m.Revoke(context.Background(), scope.Hash())
		cleanupZitadelProject(orgID, d1.ProjectID)
	})

	countKeys := func() int {
		resp, err := zitadelClient.Management().ListMachineKeys(orgCtx, &management.ListMachineKeysRequest{UserId: d1.UserID}) //nolint:staticcheck // SA1019
		if err != nil {
			t.Fatalf("listing machine keys: %v", err)
		}
		return len(resp.GetResult())
	}

	// Second Ensure triggers the rotation: two keys coexist.
	d2, err := m.Ensure(ctx, scope)
	if err != nil {
		t.Fatalf("rotation ensure: %v", err)
	}
	if d2.KeyID == d1.KeyID {
		t.Fatalf("expected rotated key, still %s", d1.KeyID)
	}
	if got := countKeys(); got != 2 {
		t.Fatalf("expected 2 live keys during rotation overlap, got %d", got)
	}
	t.Logf("rotation overlap: keys %s (old) + %s (new) both live", d1.KeyID, d2.KeyID)

	// After the grace, a sweep (with the scope still live) revokes the old key.
	time.Sleep(m.RotationGrace + time.Second)
	if err := m.Sweep(ctx, map[string]bool{scope.Hash(): true}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := countKeys(); got != 1 {
		t.Fatalf("expected 1 key after grace revocation, got %d", got)
	}
	t.Log("rotation completed: old key revoked after grace")
}

// TestDelegation_EagerRevoke_OnMapRemoval (S-234): removing the scope map
// (after its tenant CRs are gone) revokes the delegate SA and deletes its
// Secret via the eager sweep.
func TestDelegation_EagerRevoke_OnMapRemoval(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)

	nsName := fmt.Sprintf("v018-revoke-%d", ts)
	projectName := fmt.Sprintf("v018-revokeproj-%d", ts)
	createNamespace(t, ctx, nsName, nil)

	m := &zitadelv1alpha2.ScopeMap{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("v018-map-revoke-%d", ts), Namespace: operatorNamespace},
		Spec: zitadelv1alpha2.ScopeMapSpec{
			Instance:       cfg.Domain,
			Organization:   orgName,
			OrganizationId: orgID,
			Rules: []zitadelv1alpha2.ScopeMapRule{{
				Name:       "revoke-tenant",
				Namespaces: []string{nsName},
				Project:    projectName,
			}},
		},
	}
	if err := k8sClient.Create(ctx, m); err != nil {
		t.Fatalf("creating map: %v", err)
	}

	app := &zitadelv1alpha2.OIDCApp{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("v018-revokeapp-%d", ts), Namespace: nsName},
		Spec: zitadelv1alpha2.OIDCAppSpec{
			Type:         "confidential",
			AuthMethod:   "basic",
			RedirectUris: []string{"https://v018.example.com/callback"},
			SecretRef:    zitadelv1alpha2.SecretRefSpec{Name: fmt.Sprintf("v018-revokesecret-%d", ts)},
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating OIDCApp: %v", err)
	}

	var reconciled zitadelv1alpha2.OIDCApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciled, 60*time.Second)
	sec := findDelegationSecret(t, ctx, projectName)
	delegateUserID := string(sec.Data["user_id"])
	secretName := sec.Name
	projectID := reconciled.Status.ProjectId
	t.Cleanup(func() {
		cleanupZitadelProject(orgID, projectID)
		cleanupDelegate(t, orgID, delegateUserID, secretName)
	})

	// De-provision in the correct order: tenant CR first, then the map.
	cleanupResource(t, app)
	if err := k8sClient.Delete(ctx, m); err != nil {
		t.Fatalf("deleting map: %v", err)
	}

	// The map-deletion reconcile triggers the eager sweep: delegate + Secret
	// must disappear.
	deadline := time.Now().Add(45 * time.Second)
	for {
		var s corev1.Secret
		secretGone := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: operatorNamespace}, &s) != nil
		if secretGone && delegateGone(ctx, orgID, delegateUserID) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for eager revoke (secretGone=%v, delegateGone=%v)",
				secretGone, delegateGone(ctx, orgID, delegateUserID))
		}
		time.Sleep(1 * time.Second)
	}
	t.Logf("eager revoke verified: delegate %s and Secret %s removed after map deletion", delegateUserID, secretName)
}
