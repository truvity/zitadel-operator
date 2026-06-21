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
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
)

// --- DefaultLockoutPolicy ---

func TestDefaultLockoutPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("deflockout-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.DefaultLockoutPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultLockoutPolicySpec{
			LockoutPolicyFields: zitadelv1alpha2.LockoutPolicyFields{
				MaxPasswordAttempts: 5,
				MaxOtpAttempts:      3,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultLockoutPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultLockoutPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("DefaultLockoutPolicy reconciled: ready=%v", reconciled.Status.Ready)

	// Mutate.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	cr.Spec.MaxPasswordAttempts = 10
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultLockoutPolicy{}, 30*time.Second)
	t.Log("DefaultLockoutPolicy lifecycle complete")
}

// --- DefaultPasswordComplexityPolicy ---

func TestDefaultPasswordComplexityPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("defpwcplx-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.DefaultPasswordComplexityPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultPasswordComplexityPolicySpec{
			PasswordComplexityPolicyFields: zitadelv1alpha2.PasswordComplexityPolicyFields{
				MinLength:    10,
				HasLowercase: true,
				HasUppercase: true,
				HasNumber:    true,
				HasSymbol:    true,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultPasswordComplexityPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultPasswordComplexityPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("DefaultPasswordComplexityPolicy reconciled: ready=%v", reconciled.Status.Ready)

	// Mutate.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	cr.Spec.MinLength = 12
	cr.Spec.HasSymbol = false
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultPasswordComplexityPolicy{}, 30*time.Second)
	t.Log("DefaultPasswordComplexityPolicy lifecycle complete")
}

// --- DefaultPasswordAgePolicy ---

func TestDefaultPasswordAgePolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("defpwage-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.DefaultPasswordAgePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultPasswordAgePolicySpec{
			PasswordAgePolicyFields: zitadelv1alpha2.PasswordAgePolicyFields{
				MaxAgeDays:     90,
				ExpireWarnDays: 14,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultPasswordAgePolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultPasswordAgePolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("DefaultPasswordAgePolicy reconciled: ready=%v", reconciled.Status.Ready)

	// Mutate: change MaxAgeDays.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	cr.Spec.MaxAgeDays = 60
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultPasswordAgePolicy{}, 30*time.Second)
	t.Log("DefaultPasswordAgePolicy lifecycle complete")
}

// --- DefaultNotificationPolicy ---

func TestDefaultNotificationPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("defnotif-%d", time.Now().UnixMilli())

	passwordChange := true
	cr := &zitadelv1alpha2.DefaultNotificationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultNotificationPolicySpec{
			NotificationPolicyFields: zitadelv1alpha2.NotificationPolicyFields{
				PasswordChange: &passwordChange,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultNotificationPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultNotificationPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("DefaultNotificationPolicy reconciled: ready=%v", reconciled.Status.Ready)

	// Mutate: toggle PasswordChange.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	passwordChangeFalse := false
	cr.Spec.PasswordChange = &passwordChangeFalse
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultNotificationPolicy{}, 30*time.Second)
	t.Log("DefaultNotificationPolicy lifecycle complete")
}

// --- DefaultLabelPolicy ---

func TestDefaultLabelPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("deflabel-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.DefaultLabelPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultLabelPolicySpec{
			LabelPolicyFields: zitadelv1alpha2.LabelPolicyFields{
				PrimaryColor:    "#1a73e8",
				BackgroundColor: "#f8f9fa",
				WarnColor:       "#ea4335",
				FontColor:       "#202124",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultLabelPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultLabelPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("DefaultLabelPolicy reconciled: ready=%v", reconciled.Status.Ready)

	// Mutate: change PrimaryColor.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	cr.Spec.PrimaryColor = "#ff5733"
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultLabelPolicy{}, 30*time.Second)
	t.Log("DefaultLabelPolicy lifecycle complete")
}

// --- Singleton Drift Detection ---

func TestDefaultLockoutPolicy_DriftDetection(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("drift-lockout-%d", time.Now().UnixMilli())

	// Create with maxPasswordAttempts=5.
	cr := &zitadelv1alpha2.DefaultLockoutPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultLockoutPolicySpec{
			LockoutPolicyFields: zitadelv1alpha2.LockoutPolicyFields{
				MaxPasswordAttempts: 5,
				MaxOtpAttempts:      0,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultLockoutPolicy: %v", err)
	}
	var reconciled zitadelv1alpha2.DefaultLockoutPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)

	// Externally change the instance lockout policy via Zitadel Admin API directly.
	_, err := zitadelClient.Admin().UpdateLockoutPolicy(ctx, &admin.UpdateLockoutPolicyRequest{
		MaxPasswordAttempts: 99,
		MaxOtpAttempts:      0,
	})
	if err != nil {
		t.Fatalf("external drift update: %v", err)
	}

	// Trigger requeue by making a spec change (label changes don't bump generation).
	// Change MaxOtpAttempts temporarily to force a generation bump and reconcile.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting CR for spec trigger: %v", err)
	}
	cr.Spec.LockoutPolicyFields.MaxOtpAttempts = 1 // temporary change to force reconcile
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating CR spec: %v", err)
	}
	time.Sleep(3 * time.Second)

	// Now set it back to 0 and let it reconcile again (this ensures drift is detected for MaxPasswordAttempts).
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting CR for revert: %v", err)
	}
	cr.Spec.LockoutPolicyFields.MaxOtpAttempts = 0
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("reverting CR spec: %v", err)
	}
	time.Sleep(5 * time.Second)

	// Verify the operator detected drift and reconciled back to spec value.
	resp, err := zitadelClient.Admin().GetLockoutPolicy(ctx, &admin.GetLockoutPolicyRequest{})
	if err != nil {
		t.Fatalf("getting lockout policy: %v", err)
	}
	if resp.GetPolicy().GetMaxPasswordAttempts() != 5 {
		t.Fatalf("drift not corrected: expected 5, got %d", resp.GetPolicy().GetMaxPasswordAttempts())
	}
	t.Log("drift detected and corrected")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultLockoutPolicy{}, 30*time.Second)
}

// --- Singleton Duplicate Creation ---

func TestDefaultLockoutPolicy_DuplicateCreation(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()

	// Create first DefaultLockoutPolicy.
	cr1 := &zitadelv1alpha2.DefaultLockoutPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("dup1-%d", ts), Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultLockoutPolicySpec{
			LockoutPolicyFields: zitadelv1alpha2.LockoutPolicyFields{
				MaxPasswordAttempts: 3,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr1); err != nil {
		t.Fatalf("creating first DefaultLockoutPolicy: %v", err)
	}
	var r1 zitadelv1alpha2.DefaultLockoutPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr1), &r1, 30*time.Second)

	// Create second DefaultLockoutPolicy (both manage the same singleton).
	cr2 := &zitadelv1alpha2.DefaultLockoutPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("dup2-%d", ts), Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultLockoutPolicySpec{
			LockoutPolicyFields: zitadelv1alpha2.LockoutPolicyFields{
				MaxPasswordAttempts: 7,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr2); err != nil {
		t.Fatalf("creating second DefaultLockoutPolicy: %v", err)
	}

	// Wait for the second CR to get DuplicateSingleton condition.
	time.Sleep(5 * time.Second)
	var r2 zitadelv1alpha2.DefaultLockoutPolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr2), &r2); err != nil {
		t.Fatalf("getting cr2: %v", err)
	}

	// Second CR should have Ready=false with DuplicateSingleton reason.
	if r2.Status.Ready {
		t.Fatal("expected second CR to have Ready=false (DuplicateSingleton)")
	}
	foundDuplicateCondition := false
	for _, c := range r2.Status.Conditions {
		if c.Type == "Ready" && c.Reason == "DuplicateSingleton" {
			foundDuplicateCondition = true
			break
		}
	}
	if !foundDuplicateCondition {
		t.Fatal("expected DuplicateSingleton condition on second CR")
	}

	// First CR should still be Ready=true.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr1), &r1); err != nil {
		t.Fatalf("getting cr1: %v", err)
	}
	if !r1.Status.Ready {
		t.Fatal("expected first CR to remain Ready=true")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr1)
	_ = k8sClient.Delete(ctx, cr2)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr1), &zitadelv1alpha2.DefaultLockoutPolicy{}, 30*time.Second)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr2), &zitadelv1alpha2.DefaultLockoutPolicy{}, 30*time.Second)
	t.Log("duplicate creation test complete — earliest CR wins, duplicate gets DuplicateSingleton")
}

// --- Reset-on-Delete Annotation ---

func TestDefaultLockoutPolicy_ResetOnDeleteAnnotation(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("reset-lockout-%d", time.Now().UnixMilli())

	// Create with reset-on-delete annotation and custom values.
	cr := &zitadelv1alpha2.DefaultLockoutPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Annotations: map[string]string{
				"zitadel.truvity.io/reset-on-delete": "true",
			},
		},
		Spec: zitadelv1alpha2.DefaultLockoutPolicySpec{
			LockoutPolicyFields: zitadelv1alpha2.LockoutPolicyFields{
				MaxPasswordAttempts: 10,
				MaxOtpAttempts:      5,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultLockoutPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultLockoutPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)

	// Verify the custom values are applied.
	resp, err := zitadelClient.Admin().GetLockoutPolicy(ctx, &admin.GetLockoutPolicyRequest{})
	if err != nil {
		t.Fatalf("getting lockout policy: %v", err)
	}
	if resp.GetPolicy().GetMaxPasswordAttempts() != 10 {
		t.Fatalf("expected maxPasswordAttempts=10, got %d", resp.GetPolicy().GetMaxPasswordAttempts())
	}

	// Delete — should reset to defaults because annotation is present.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultLockoutPolicy{}, 30*time.Second)

	// Verify the policy was reset to Zitadel defaults (0, 0).
	resp, err = zitadelClient.Admin().GetLockoutPolicy(ctx, &admin.GetLockoutPolicyRequest{})
	if err != nil {
		t.Fatalf("getting lockout policy after delete: %v", err)
	}
	if resp.GetPolicy().GetMaxPasswordAttempts() != 0 {
		t.Fatalf("expected maxPasswordAttempts=0 after reset-on-delete, got %d", resp.GetPolicy().GetMaxPasswordAttempts())
	}
	if resp.GetPolicy().GetMaxOtpAttempts() != 0 {
		t.Fatalf("expected maxOtpAttempts=0 after reset-on-delete, got %d", resp.GetPolicy().GetMaxOtpAttempts())
	}
	t.Log("reset-on-delete annotation test complete — policy reset to defaults on delete")
}

// --- Delete Without Annotation (leave-as-is) ---

func TestDefaultLockoutPolicy_DeleteWithoutAnnotation(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("noanno-lockout-%d", time.Now().UnixMilli())

	// Create WITHOUT reset-on-delete annotation.
	cr := &zitadelv1alpha2.DefaultLockoutPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultLockoutPolicySpec{
			LockoutPolicyFields: zitadelv1alpha2.LockoutPolicyFields{
				MaxPasswordAttempts: 8,
				MaxOtpAttempts:      4,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultLockoutPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultLockoutPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)

	// Verify the custom values are applied.
	resp, err := zitadelClient.Admin().GetLockoutPolicy(ctx, &admin.GetLockoutPolicyRequest{})
	if err != nil {
		t.Fatalf("getting lockout policy: %v", err)
	}
	if resp.GetPolicy().GetMaxPasswordAttempts() != 8 {
		t.Fatalf("expected maxPasswordAttempts=8, got %d", resp.GetPolicy().GetMaxPasswordAttempts())
	}

	// Delete — should NOT reset (no annotation).
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultLockoutPolicy{}, 30*time.Second)

	// Verify the policy was LEFT AS-IS (still 8, 4).
	resp, err = zitadelClient.Admin().GetLockoutPolicy(ctx, &admin.GetLockoutPolicyRequest{})
	if err != nil {
		t.Fatalf("getting lockout policy after delete: %v", err)
	}
	if resp.GetPolicy().GetMaxPasswordAttempts() != 8 {
		t.Fatalf("expected maxPasswordAttempts=8 (left as-is), got %d", resp.GetPolicy().GetMaxPasswordAttempts())
	}
	t.Log("delete without annotation test complete — policy left as-is")
}
