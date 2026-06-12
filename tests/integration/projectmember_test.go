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

func TestProjectMember_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("pmproj-%d", ts)
	muName := fmt.Sprintf("pmmu-%d", ts)
	userName := fmt.Sprintf("pmbot-%d", ts)
	secretName := fmt.Sprintf("pmkey-%d", ts)
	memberName := fmt.Sprintf("pmember-%d", ts)

	// Create Project.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project: %v", err)
	}
	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)
	t.Logf("project ready: %s (id: %s)", projName, reconciledProj.Status.ProjectId)

	// Create MachineUser.
	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{Name: muName, Namespace: "default"},
		Spec: zitadelv1alpha2.MachineUserSpec{
			UserName:     userName,
			KeySecretRef: zitadelv1alpha2.MachineKeySecretRef{Name: secretName},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser: %v", err)
	}
	var reconciledMU zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciledMU, 30*time.Second)
	t.Logf("machine user ready: %s (id: %s)", muName, reconciledMU.Status.UserId)

	// Create ProjectMember referencing both.
	pm := &zitadelv1alpha2.ProjectMember{
		ObjectMeta: metav1.ObjectMeta{Name: memberName, Namespace: "default"},
		Spec: zitadelv1alpha2.ProjectMemberSpec{
			ProjectRef: &zitadelv1alpha2.ResourceRef{Name: projName},
			UserRef:    &zitadelv1alpha2.ResourceRef{Name: muName},
			Roles:      []string{"PROJECT_OWNER"},
		},
	}
	if err := k8sClient.Create(ctx, pm); err != nil {
		t.Fatalf("creating ProjectMember: %v", err)
	}

	var reconciledPM zitadelv1alpha2.ProjectMember
	waitForReady(t, ctx, client.ObjectKeyFromObject(pm), &reconciledPM, 30*time.Second)

	t.Logf("project member reconciled: ready=%v", reconciledPM.Status.Ready)

	if !reconciledPM.Status.Ready {
		t.Fatal("expected ready=true")
	}

	// Cleanup (member → machine user → project).
	_ = k8sClient.Delete(ctx, pm)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(pm), &zitadelv1alpha2.ProjectMember{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, mu)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(mu), &zitadelv1alpha2.MachineUser{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, proj)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
	t.Log("project member lifecycle complete")
}
