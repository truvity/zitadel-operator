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

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
	"google.golang.org/grpc/metadata"
)

// TestMachineUser_ScopeRoles_ConnectionBundle (S-250) covers the INF-426
// MachineUser extension end to end in a project-scoped namespace:
// spec.roles become a user grant on the scope's project (via the delegated
// client) and the key Secret is a full connection bundle.
func TestMachineUser_ScopeRoles_ConnectionBundle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgID, orgName := testOrg(t, ctx)
	orgCtx := metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	nsName := fmt.Sprintf("v018-mu-%d", ts)
	projectName := fmt.Sprintf("v018-muproj-%d", ts)
	createNamespace(t, ctx, nsName, nil)

	// Pre-create the scope project with the roles the grant will use.
	projResp, err := zitadelClient.Project().CreateProject(orgCtx, &projectv2.CreateProjectRequest{
		Name:           projectName,
		OrganizationId: orgID,
	})
	if err != nil {
		t.Fatalf("creating scope project: %v", err)
	}
	projectID := projResp.GetProjectId()
	t.Cleanup(func() { cleanupZitadelProject(orgID, projectID) })
	for _, role := range []string{"v018.reader", "v018.writer"} {
		if _, err := zitadelClient.Management().AddProjectRole(orgCtx, &management.AddProjectRoleRequest{ //nolint:staticcheck // SA1019
			ProjectId:   projectID,
			RoleKey:     role,
			DisplayName: role,
		}); err != nil {
			t.Fatalf("adding project role %s: %v", role, err)
		}
	}

	createScopeMap(t, ctx, fmt.Sprintf("v018-map-mu-%d", ts), zitadelv1alpha2.ZitadelScopeMapSpec{
		Instance:       cfg.Domain,
		Organization:   orgName,
		OrganizationId: orgID,
		Rules: []zitadelv1alpha2.ScopeMapRule{{
			Name:       "mu-tenant",
			Namespaces: []string{nsName},
			Project:    projectName,
		}},
	})

	var cleanupUserID, cleanupSecretName string
	t.Cleanup(func() { cleanupDelegate(t, orgID, cleanupUserID, cleanupSecretName) })

	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("v018-mu-%d", ts), Namespace: nsName},
		Spec: zitadelv1alpha2.MachineUserSpec{
			UserName:     fmt.Sprintf("v018-mu-%d", ts),
			Roles:        []string{"v018.reader", "v018.writer"},
			KeySecretRef: zitadelv1alpha2.MachineKeySecretRef{Name: fmt.Sprintf("v018-mu-secret-%d", ts)},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, mu) })

	var reconciled zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciled, 60*time.Second)

	if reconciled.Status.GrantId == "" || reconciled.Status.ProjectId != projectID {
		t.Fatalf("expected role grant on scope project %s, got status %+v", projectID, reconciled.Status)
	}

	// Verify the grant server-side.
	grants, err := zitadelClient.Management().ListUserGrants(orgCtx, &management.ListUserGrantRequest{}) //nolint:staticcheck // SA1019
	if err != nil {
		t.Fatalf("listing grants: %v", err)
	}
	found := false
	for _, g := range grants.GetResult() {
		if g.GetId() == reconciled.Status.GrantId {
			found = true
			if len(g.GetRoleKeys()) != 2 {
				t.Fatalf("expected 2 granted roles, got %v", g.GetRoleKeys())
			}
		}
	}
	if !found {
		t.Fatalf("grant %s not found in Zitadel", reconciled.Status.GrantId)
	}

	// Connection bundle: a consumer must be able to build a client from the
	// Secret alone.
	var sec corev1.Secret
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: mu.Spec.KeySecretRef.Name, Namespace: nsName}, &sec); err != nil {
		t.Fatalf("reading bundle Secret: %v", err)
	}
	for _, key := range []string{"key.json", "instanceUrl", "issuer", "orgId", "projectId"} {
		if len(sec.Data[key]) == 0 {
			t.Fatalf("connection bundle missing %q (have keys: %v)", key, secretKeys(&sec))
		}
	}
	if string(sec.Data["orgId"]) != orgID || string(sec.Data["projectId"]) != projectID {
		t.Fatalf("bundle org/project mismatch: %s/%s", sec.Data["orgId"], sec.Data["projectId"])
	}
	t.Logf("bundle keys: %v; instanceId present: %v", secretKeys(&sec), len(sec.Data["instanceId"]) > 0)

	// Narrowing the roles must update the grant.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mu), &reconciled); err != nil {
		t.Fatal(err)
	}
	reconciled.Spec.Roles = []string{"v018.reader"}
	if err := k8sClient.Update(ctx, &reconciled); err != nil {
		t.Fatalf("narrowing roles: %v", err)
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		grants, err := zitadelClient.Management().ListUserGrants(orgCtx, &management.ListUserGrantRequest{}) //nolint:staticcheck // SA1019
		if err == nil {
			for _, g := range grants.GetResult() {
				if g.GetId() == reconciled.Status.GrantId && len(g.GetRoleKeys()) == 1 && g.GetRoleKeys()[0] == "v018.reader" {
					goto narrowed
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for grant to narrow to [v018.reader]")
		}
		time.Sleep(1 * time.Second)
	}
narrowed:
	// Record delegate identity for cleanup.
	dsec := findDelegationSecret(t, ctx, projectName)
	cleanupUserID, cleanupSecretName = string(dsec.Data["user_id"]), dsec.Name
	t.Log("MachineUser scope roles + connection bundle verified; narrowing applied")
}

// TestMachineUser_KeyRotation_DualKey (S-251) proves spec.key.rotateAfter:
// dual-key rotation with a grace overlap, driven by the controller in
// passthrough mode. Backward compatibility (no key.rotateAfter = never
// rotate) is covered by every pre-existing MachineUser test.
func TestMachineUser_KeyRotation_DualKey(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()

	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("v018-rotmu-%d", ts), Namespace: "default"},
		Spec: zitadelv1alpha2.MachineUserSpec{
			OrganizationId: testOrgID,
			UserName:       fmt.Sprintf("v018-rotmu-%d", ts),
			Key: &zitadelv1alpha2.MachineKeySpec{
				RotateAfter:   &metav1.Duration{Duration: 1 * time.Second},
				RotationGrace: &metav1.Duration{Duration: 5 * time.Second},
			},
			KeySecretRef: zitadelv1alpha2.MachineKeySecretRef{Name: fmt.Sprintf("v018-rotmu-secret-%d", ts)},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser: %v", err)
	}
	t.Cleanup(func() { cleanupResource(t, mu) })

	var reconciled zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciled, 60*time.Second)
	if reconciled.Status.KeyId == "" || reconciled.Status.KeyCreatedAt == nil {
		t.Fatalf("expected key bookkeeping in status, got %+v", reconciled.Status)
	}
	firstKeyID := reconciled.Status.KeyId

	var firstKeyJSON []byte
	var sec corev1.Secret
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: mu.Spec.KeySecretRef.Name, Namespace: "default"}, &sec); err != nil {
		t.Fatal(err)
	}
	firstKeyJSON = append(firstKeyJSON, sec.Data["key.json"]...)

	orgCtx := metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", testOrgID)
	countKeys := func() int {
		resp, err := zitadelClient.Management().ListMachineKeys(orgCtx, &management.ListMachineKeysRequest{UserId: reconciled.Status.UserId}) //nolint:staticcheck // SA1019
		if err != nil {
			t.Fatalf("listing machine keys: %v", err)
		}
		return len(resp.GetResult())
	}

	// Trigger a reconcile after the rotation cycle elapsed.
	time.Sleep(2 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mu), &reconciled); err != nil {
		t.Fatal(err)
	}
	reconciled.Spec.Description = "poke rotation"
	if err := k8sClient.Update(ctx, &reconciled); err != nil {
		t.Fatal(err)
	}

	// Rotation: new key in Secret + status, old key still live (overlap).
	deadline := time.Now().Add(45 * time.Second)
	for {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mu), &reconciled); err == nil &&
			reconciled.Status.KeyId != "" && reconciled.Status.KeyId != firstKeyID {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for key rotation (still %s)", firstKeyID)
		}
		time.Sleep(1 * time.Second)
	}
	if reconciled.Status.PreviousKeyId != firstKeyID || reconciled.Status.PreviousKeyRevokeAt == nil {
		t.Fatalf("expected previous-key bookkeeping, got %+v", reconciled.Status)
	}
	if got := countKeys(); got != 2 {
		t.Fatalf("expected 2 live keys during grace overlap, got %d", got)
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: mu.Spec.KeySecretRef.Name, Namespace: "default"}, &sec); err != nil {
		t.Fatal(err)
	}
	if string(sec.Data["key.json"]) == string(firstKeyJSON) {
		t.Fatal("Secret still holds the pre-rotation key")
	}
	t.Logf("rotation overlap: keys %s (old) + %s (new) live", firstKeyID, reconciled.Status.KeyId)

	// Freeze further rotations (a 1s test cycle would otherwise rotate on
	// every requeue) so the pending revocation can be observed.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mu), &reconciled); err != nil {
		t.Fatal(err)
	}
	reconciled.Spec.Key = nil
	if err := k8sClient.Update(ctx, &reconciled); err != nil {
		t.Fatal(err)
	}

	// After the grace the controller revokes the old key (short self-requeue).
	deadline = time.Now().Add(60 * time.Second)
	for {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mu), &reconciled); err == nil &&
			reconciled.Status.PreviousKeyId == "" && countKeys() == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for old-key revocation (keys=%d, prev=%q)", countKeys(), reconciled.Status.PreviousKeyId)
		}
		time.Sleep(2 * time.Second)
	}
	t.Log("dual-key rotation completed: old key revoked after grace, one key live")
}

func secretKeys(s *corev1.Secret) []string {
	keys := make([]string, 0, len(s.Data))
	for k := range s.Data {
		keys = append(keys, k)
	}
	return keys
}
