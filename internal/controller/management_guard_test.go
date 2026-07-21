package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
)

// newGuardFakeK8s builds a fake client with the zitadel scheme and status
// subresource enabled for the Project kind used in these tests.
func newGuardFakeK8s(t *testing.T, objs ...runtime.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = zitadelv1alpha2.AddToScheme(scheme)
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&zitadelv1alpha2.Project{}).
		Build()
}

func guardTestConfig() *config.Config {
	return &config.Config{
		Domain:              "auth.example.com",
		InstanceAlias:       "prod",
		Binding:             config.BindingOrgOwner,
		BoundOrganizationId: "org-1",
		OperatorNamespace:   "op-ns",
	}
}

func TestManagementIdentity(t *testing.T) {
	cfg := guardTestConfig()
	if got, want := cfg.ManagementIdentity(), "prod/org/org-1"; got != want {
		t.Fatalf("org-owner identity = %q, want %q", got, want)
	}

	cfg.Binding = config.BindingIAMOwner
	if got, want := cfg.ManagementIdentity(), "prod/ns/op-ns"; got != want {
		t.Fatalf("iam-owner identity = %q, want %q", got, want)
	}

	// org-owner before startup verification (no bound org yet) falls back to
	// the namespace form rather than producing an empty discriminator.
	cfg.Binding = config.BindingOrgOwner
	cfg.BoundOrganizationId = ""
	if got, want := cfg.ManagementIdentity(), "prod/ns/op-ns"; got != want {
		t.Fatalf("unverified org-owner identity = %q, want %q", got, want)
	}
}

func TestManagementGate_StampsAnnotationOnAdoption(t *testing.T) {
	cfg := guardTestConfig()
	cr := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
	}
	k8s := newGuardFakeK8s(t, cr)

	var fresh zitadelv1alpha2.Project
	if err := k8s.Get(context.Background(), client.ObjectKeyFromObject(cr), &fresh); err != nil {
		t.Fatal(err)
	}
	done, _, err := managementGate(context.Background(), k8s, cfg, &fresh, &fresh.Status.Conditions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done {
		t.Fatal("expected the gate to let the adopting operator proceed")
	}

	var after zitadelv1alpha2.Project
	if err := k8s.Get(context.Background(), client.ObjectKeyFromObject(cr), &after); err != nil {
		t.Fatal(err)
	}
	if got := after.Annotations[AnnotationManagedBy]; got != cfg.ManagementIdentity() {
		t.Fatalf("managed-by annotation = %q, want %q", got, cfg.ManagementIdentity())
	}
}

func TestManagementGate_OwnerProceeds(t *testing.T) {
	cfg := guardTestConfig()
	cr := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: "p", Namespace: "default",
			Annotations: map[string]string{AnnotationManagedBy: cfg.ManagementIdentity()},
		},
	}
	k8s := newGuardFakeK8s(t, cr)

	var fresh zitadelv1alpha2.Project
	if err := k8s.Get(context.Background(), client.ObjectKeyFromObject(cr), &fresh); err != nil {
		t.Fatal(err)
	}
	done, _, err := managementGate(context.Background(), k8s, cfg, &fresh, &fresh.Status.Conditions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done {
		t.Fatal("expected the owning operator to proceed")
	}
}

func TestManagementGate_ForeignSkipsAndSetsCondition(t *testing.T) {
	cfg := guardTestConfig()
	cr := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: "p", Namespace: "default",
			Annotations: map[string]string{AnnotationManagedBy: "prod/org/org-OTHER"},
		},
	}
	k8s := newGuardFakeK8s(t, cr)

	var fresh zitadelv1alpha2.Project
	if err := k8s.Get(context.Background(), client.ObjectKeyFromObject(cr), &fresh); err != nil {
		t.Fatal(err)
	}
	done, result, err := managementGate(context.Background(), k8s, cfg, &fresh, &fresh.Status.Conditions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !done {
		t.Fatal("expected the foreign operator to skip reconciliation")
	}
	if result.RequeueAfter != requeueInterval {
		t.Fatalf("expected periodic requeue, got %v", result.RequeueAfter)
	}

	var after zitadelv1alpha2.Project
	if err := k8s.Get(context.Background(), client.ObjectKeyFromObject(cr), &after); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range after.Status.Conditions {
		if c.Type == ConditionTypeForeignManager {
			found = true
			if c.Status != metav1.ConditionTrue || c.Reason != "ManagedByOtherOperator" {
				t.Fatalf("unexpected condition: %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("expected ForeignManager condition, got %+v", after.Status.Conditions)
	}
	// The annotation must remain the owner's — no takeover.
	if got := after.Annotations[AnnotationManagedBy]; got != "prod/org/org-OTHER" {
		t.Fatalf("annotation changed to %q, guard must not take ownership", got)
	}
}

func TestManagementGate_ForeignSkipsDuringDeletion(t *testing.T) {
	cfg := guardTestConfig()
	now := metav1.Now()
	cr := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: "p", Namespace: "default",
			Annotations:       map[string]string{AnnotationManagedBy: "prod/org/org-OTHER"},
			DeletionTimestamp: &now,
			Finalizers:        []string{finalizerName},
		},
	}
	k8s := newGuardFakeK8s(t, cr)

	var fresh zitadelv1alpha2.Project
	if err := k8s.Get(context.Background(), client.ObjectKeyFromObject(cr), &fresh); err != nil {
		t.Fatal(err)
	}
	done, _, err := managementGate(context.Background(), k8s, cfg, &fresh, &fresh.Status.Conditions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !done {
		t.Fatal("a same-instance foreign delete is not a no-op: the gate must hold during deletion")
	}
}

func TestManagementGate_UnstampedDeletionProceedsWithoutStamping(t *testing.T) {
	cfg := guardTestConfig()
	now := metav1.Now()
	cr := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: "p", Namespace: "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{finalizerName},
		},
	}
	k8s := newGuardFakeK8s(t, cr)

	var fresh zitadelv1alpha2.Project
	if err := k8s.Get(context.Background(), client.ObjectKeyFromObject(cr), &fresh); err != nil {
		t.Fatal(err)
	}
	done, _, err := managementGate(context.Background(), k8s, cfg, &fresh, &fresh.Status.Conditions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done {
		t.Fatal("legacy (never-adopted) CRs must be deletable")
	}

	var after zitadelv1alpha2.Project
	if err := k8s.Get(context.Background(), client.ObjectKeyFromObject(cr), &after); err != nil {
		t.Fatal(err)
	}
	if _, stamped := after.Annotations[AnnotationManagedBy]; stamped {
		t.Fatal("gate must never adopt a CR on its way out")
	}
}

func TestManagementGate_OwnerReleasesStaleCondition(t *testing.T) {
	cfg := guardTestConfig()
	cr := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: "p", Namespace: "default",
			Annotations: map[string]string{AnnotationManagedBy: cfg.ManagementIdentity()},
		},
		Status: zitadelv1alpha2.ProjectStatus{
			Conditions: []metav1.Condition{{
				Type:               ConditionTypeForeignManager,
				Status:             metav1.ConditionTrue,
				Reason:             "ManagedByOtherOperator",
				Message:            "stale",
				LastTransitionTime: metav1.Now(),
			}},
		},
	}
	k8s := newGuardFakeK8s(t, cr)

	var fresh zitadelv1alpha2.Project
	if err := k8s.Get(context.Background(), client.ObjectKeyFromObject(cr), &fresh); err != nil {
		t.Fatal(err)
	}
	done, _, err := managementGate(context.Background(), k8s, cfg, &fresh, &fresh.Status.Conditions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done {
		t.Fatal("expected the (new) owner to proceed")
	}
	// Note: with SSA the release only removes the guard manager's own entry;
	// the fake client mirrors that closely enough for this smoke check, but
	// the authoritative both-directions coverage lives in the integration
	// suite (foreignmanager_test.go).
}
