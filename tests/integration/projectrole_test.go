//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

// listRoleKeys returns the role keys currently present on the project.
func listRoleKeys(t *testing.T, ctx context.Context, projectID, key string) []string {
	t.Helper()
	resp, err := zitadelClient.Project().ListProjectRoles(ctx, &projectv2.ListProjectRolesRequest{
		ProjectId: projectID,
		Filters: []*projectv2.ProjectRoleSearchFilter{
			{
				Filter: &projectv2.ProjectRoleSearchFilter_RoleKeyFilter{
					RoleKeyFilter: &projectv2.ProjectRoleKeyFilter{
						Key:    key,
						Method: filterv2.TextFilterMethod_TEXT_FILTER_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("listing project roles: %v", err)
	}
	keys := make([]string, 0, len(resp.GetProjectRoles()))
	for _, r := range resp.GetProjectRoles() {
		keys = append(keys, r.GetKey())
	}
	return keys
}

func TestProjectRole_WithProjectRef(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("roleref-proj-%d", ts)
	roleName := fmt.Sprintf("roleref-role-%d", ts)

	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
		Spec:       zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, proj)
		}
	})

	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)

	// ProjectRole via projectRef with explicit key, displayName and group.
	role := &zitadelv1alpha2.ProjectRole{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: "default"},
		Spec: zitadelv1alpha2.ProjectRoleSpec{
			ProjectRef:  &zitadelv1alpha2.ResourceRef{Name: projName},
			Key:         "default:reader",
			DisplayName: "Reader",
			Group:       "default",
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("creating ProjectRole CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, role)
		}
	})

	var reconciledRole zitadelv1alpha2.ProjectRole
	waitForReady(t, ctx, client.ObjectKeyFromObject(role), &reconciledRole, 30*time.Second)

	if reconciledRole.Status.ProjectId != reconciledProj.Status.ProjectId {
		t.Fatalf("expected status.projectId=%s, got %s", reconciledProj.Status.ProjectId, reconciledRole.Status.ProjectId)
	}
	if reconciledRole.Status.Key != "default:reader" {
		t.Fatalf("expected status.key=default:reader, got %s", reconciledRole.Status.Key)
	}
	if reconciledRole.Status.ObservedGeneration != reconciledRole.Generation {
		t.Fatalf("expected observedGeneration=%d, got %d", reconciledRole.Generation, reconciledRole.Status.ObservedGeneration)
	}

	// Role exists in Zitadel.
	if keys := listRoleKeys(t, ctx, reconciledProj.Status.ProjectId, "default:reader"); len(keys) != 1 {
		t.Fatalf("expected role default:reader in Zitadel, got %v", keys)
	}

	// Deleting the ProjectRole removes the role from the project.
	if err := k8sClient.Delete(ctx, role); err != nil {
		t.Fatalf("deleting ProjectRole CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(role), &zitadelv1alpha2.ProjectRole{}, 30*time.Second)
	if keys := listRoleKeys(t, ctx, reconciledProj.Status.ProjectId, "default:reader"); len(keys) != 0 {
		t.Fatalf("expected role removed from Zitadel, still present: %v", keys)
	}
	t.Log("projectrole with projectRef lifecycle complete")
}

func TestProjectRole_WithRawProjectIdAndDefaults(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("roleid-proj-%d", ts)
	// The CR name doubles as the role key (spec.key default).
	roleName := fmt.Sprintf("roleid-role-%d", ts)

	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
		Spec:       zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, proj)
		}
	})
	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)

	role := &zitadelv1alpha2.ProjectRole{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: "default"},
		Spec: zitadelv1alpha2.ProjectRoleSpec{
			ProjectId: reconciledProj.Status.ProjectId,
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("creating ProjectRole CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, role)
		}
	})

	var reconciledRole zitadelv1alpha2.ProjectRole
	waitForReady(t, ctx, client.ObjectKeyFromObject(role), &reconciledRole, 30*time.Second)

	if reconciledRole.Status.Key != roleName {
		t.Fatalf("expected key to default to the CR name %q, got %q", roleName, reconciledRole.Status.Key)
	}
	if keys := listRoleKeys(t, ctx, reconciledProj.Status.ProjectId, roleName); len(keys) != 1 {
		t.Fatalf("expected role %s in Zitadel, got %v", roleName, keys)
	}
	t.Log("projectrole with raw projectId and defaulted key complete")
}

func TestProjectRole_RefNotReadyThenReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("rolewait-proj-%d", ts)
	roleName := fmt.Sprintf("rolewait-role-%d", ts)

	// ProjectRole first — the referenced Project does not exist yet.
	role := &zitadelv1alpha2.ProjectRole{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: "default"},
		Spec: zitadelv1alpha2.ProjectRoleSpec{
			ProjectRef: &zitadelv1alpha2.ResourceRef{Name: projName},
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("creating ProjectRole CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, role)
		}
	})

	// Expect a polite ProjectNotReady wait, not an error loop.
	deadline := time.Now().Add(20 * time.Second)
	sawWaiting := false
	for time.Now().Before(deadline) {
		var cur zitadelv1alpha2.ProjectRole
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(role), &cur); err == nil {
			for _, c := range cur.Status.Conditions {
				if c.Type == "Ready" && c.Status == metav1.ConditionFalse && c.Reason == "ProjectNotReady" {
					sawWaiting = true
				}
			}
		}
		if sawWaiting {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !sawWaiting {
		t.Fatal("expected Ready=False/ProjectNotReady while the Project CR is missing")
	}

	// Now create the Project; the role must converge without manual help.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
		Spec:       zitadelv1alpha2.ProjectSpec{OrganizationId: testOrgID},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, proj)
		}
	})

	var reconciledRole zitadelv1alpha2.ProjectRole
	waitForReady(t, ctx, client.ObjectKeyFromObject(role), &reconciledRole, 60*time.Second)
	if reconciledRole.Status.ProjectId == "" {
		t.Fatal("expected resolved projectId after the Project became ready")
	}
	t.Log("projectrole not-ready requeue converged")
}

func TestProjectRole_MutualExclusionRejected(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	roleName := fmt.Sprintf("rolebad-%d", ts)

	role := &zitadelv1alpha2.ProjectRole{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: "default"},
		Spec: zitadelv1alpha2.ProjectRoleSpec{
			ProjectRef: &zitadelv1alpha2.ResourceRef{Name: "some-project"},
			ProjectId:  "123456789",
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("creating ProjectRole CR: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			cleanupResource(t, role)
		}
	})

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var cur zitadelv1alpha2.ProjectRole
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(role), &cur); err == nil {
			for _, c := range cur.Status.Conditions {
				if c.Type == "Ready" && c.Status == metav1.ConditionFalse && c.Reason == "InvalidSpec" &&
					strings.Contains(c.Message, "mutually exclusive") {
					t.Log("mutual exclusion rejected with InvalidSpec condition")
					return
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("expected Ready=False/InvalidSpec for projectRef+projectId")
}
