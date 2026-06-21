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

// --- HumanUser: org ref not ready ---

func TestHumanUser_OrgRefNotReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("hu-noorg-%d", ts)

	cr := &zitadelv1alpha2.HumanUser{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.HumanUserSpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: "nonexistent-org-hu"},
			UserName:        fmt.Sprintf("noorg-user-%d", ts),
			FirstName:       "No",
			LastName:        "Org",
			Email:           fmt.Sprintf("noorg-%d@example.com", ts),
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating HumanUser: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.HumanUser
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting HumanUser: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when org ref doesn't exist")
	}
	t.Log("HumanUser correctly not ready when org ref missing")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.HumanUser{}, 30*time.Second)
}

// --- OrgMember: user ref not ready ---

func TestOrgMember_UserRefNotReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("om-nouser-org-%d", ts)
	memberName := fmt.Sprintf("om-nouser-%d", ts)

	// Create org (so org ref resolves).
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create OrgMember with non-existent user ref.
	cr := &zitadelv1alpha2.OrgMember{
		ObjectMeta: metav1.ObjectMeta{Name: memberName, Namespace: "default"},
		Spec: zitadelv1alpha2.OrgMemberSpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			UserRef:         &zitadelv1alpha2.ResourceRef{Name: "nonexistent-user"},
			Roles:           []string{"ORG_OWNER"},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating OrgMember: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.OrgMember
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting OrgMember: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when user ref doesn't exist")
	}
	t.Log("OrgMember correctly not ready when user ref missing")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.OrgMember{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
}

// --- InstanceMember: user ref not ready ---

func TestInstanceMember_UserRefNotReady(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("im-nouser-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.InstanceMember{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.InstanceMemberSpec{
			UserRef: &zitadelv1alpha2.ResourceRef{Name: "nonexistent-user-im"},
			Roles:   []string{"IAM_ORG_MANAGER"},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating InstanceMember: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.InstanceMember
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting InstanceMember: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when user ref doesn't exist")
	}
	t.Log("InstanceMember correctly not ready when user ref missing")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.InstanceMember{}, 30*time.Second)
}

// --- SmsProvider: invalid spec (both twilio + http) ---

func TestSmsProvider_InvalidSpec_BothTwilioAndHttp(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("sms-invalid-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.SmsProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.SmsProviderSpec{
			Twilio: &zitadelv1alpha2.TwilioSmsProvider{
				SID:          "ACtest",
				SenderNumber: "+1234567890",
				TokenSecretRef: zitadelv1alpha2.SecretKeyRef{
					Name: "twilio-token",
				},
			},
			Http: &zitadelv1alpha2.HttpSmsProvider{
				Endpoint: "https://httpbin.org/post",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating SmsProvider: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.SmsProvider
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting SmsProvider: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when both twilio and http set")
	}

	readyCond := findCondition(reconciled.Status.Conditions, "Ready")
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != "InvalidSpec" {
		t.Errorf("expected reason=InvalidSpec, got %s", readyCond.Reason)
	}
	t.Logf("SmsProvider correctly rejected: reason=%s, message=%s", readyCond.Reason, readyCond.Message)

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.SmsProvider{}, 30*time.Second)
}

// --- SmsProvider Twilio: token secret not found ---

func TestSmsProvider_TwilioTokenSecretNotFound(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("sms-notoken-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.SmsProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.SmsProviderSpec{
			Twilio: &zitadelv1alpha2.TwilioSmsProvider{
				SID:          "ACtest",
				SenderNumber: "+1234567890",
				TokenSecretRef: zitadelv1alpha2.SecretKeyRef{
					Name: "nonexistent-twilio-token",
					Key:  "token",
				},
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating SmsProvider: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.SmsProvider
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting SmsProvider: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when token secret doesn't exist")
	}

	readyCond := findCondition(reconciled.Status.Conditions, "Ready")
	if readyCond != nil {
		if readyCond.Status != metav1.ConditionFalse {
			t.Errorf("expected Ready=False, got %s", readyCond.Status)
		}
		t.Logf("SmsProvider correctly not ready: reason=%s", readyCond.Reason)
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.SmsProvider{}, 30*time.Second)
}

// --- GitHubIdP: secret not found ---

func TestGitHubIdP_SecretNotFound(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("github-nosecret-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.GitHubIdP{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.GitHubIdPSpec{
			Name:     "NoSecret GitHub",
			ClientId: "Ov23liXXXXXX",
			ClientSecretRef: zitadelv1alpha2.SecretKeyRef{
				Name: "nonexistent-github-secret",
			},
			IsAutoCreation: true,
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating GitHubIdP: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.GitHubIdP
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting GitHubIdP: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when secret doesn't exist")
	}

	readyCond := findCondition(reconciled.Status.Conditions, "Ready")
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != "SecretNotFound" {
		t.Errorf("expected reason=SecretNotFound, got %s", readyCond.Reason)
	}
	t.Logf("GitHubIdP correctly not ready: reason=%s", readyCond.Reason)

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.GitHubIdP{}, 30*time.Second)
}

// --- PasswordComplexityPolicy: org ref not ready ---

func TestPasswordComplexityPolicy_OrgRefNotReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("pwdcpol-noorg-%d", ts)

	cr := &zitadelv1alpha2.PasswordComplexityPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.PasswordComplexityPolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: "nonexistent-org-pwdc"},
			PasswordComplexityPolicyFields: zitadelv1alpha2.PasswordComplexityPolicyFields{
				MinLength: 8,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating PasswordComplexityPolicy: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.PasswordComplexityPolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting PasswordComplexityPolicy: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when organizationRef doesn't exist")
	}
	t.Log("PasswordComplexityPolicy correctly not ready when org ref missing")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.PasswordComplexityPolicy{}, 30*time.Second)
}

// --- LockoutPolicy: org ref not ready ---

func TestLockoutPolicy_OrgRefNotReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("lockpol-noorg-%d", ts)

	cr := &zitadelv1alpha2.LockoutPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.LockoutPolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: "nonexistent-org-lock"},
			LockoutPolicyFields: zitadelv1alpha2.LockoutPolicyFields{
				MaxPasswordAttempts: 5,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating LockoutPolicy: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.LockoutPolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting LockoutPolicy: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when organizationRef doesn't exist")
	}
	t.Log("LockoutPolicy correctly not ready when org ref missing")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.LockoutPolicy{}, 30*time.Second)
}

// --- PasswordAgePolicy: org ref not ready ---

func TestPasswordAgePolicy_OrgRefNotReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("pwdagepol-noorg-%d", ts)

	cr := &zitadelv1alpha2.PasswordAgePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.PasswordAgePolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: "nonexistent-org-pwdage"},
			PasswordAgePolicyFields: zitadelv1alpha2.PasswordAgePolicyFields{
				MaxAgeDays: 90,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating PasswordAgePolicy: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.PasswordAgePolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting PasswordAgePolicy: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when organizationRef doesn't exist")
	}
	t.Log("PasswordAgePolicy correctly not ready when org ref missing")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.PasswordAgePolicy{}, 30*time.Second)
}

// --- NotificationPolicy: org ref not ready ---

func TestNotificationPolicy_OrgRefNotReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("notifpol-noorg-%d", ts)

	passwordChange := true
	cr := &zitadelv1alpha2.NotificationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.NotificationPolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: "nonexistent-org-notif"},
			NotificationPolicyFields: zitadelv1alpha2.NotificationPolicyFields{
				PasswordChange: &passwordChange,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating NotificationPolicy: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.NotificationPolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting NotificationPolicy: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when organizationRef doesn't exist")
	}
	t.Log("NotificationPolicy correctly not ready when org ref missing")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.NotificationPolicy{}, 30*time.Second)
}

// --- LabelPolicy: org ref not ready ---

func TestLabelPolicy_OrgRefNotReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("labelpol-noorg-%d", ts)

	cr := &zitadelv1alpha2.LabelPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.LabelPolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: "nonexistent-org-label"},
			LabelPolicyFields: zitadelv1alpha2.LabelPolicyFields{
				PrimaryColor: "#ff0000",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating LabelPolicy: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.LabelPolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting LabelPolicy: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when organizationRef doesn't exist")
	}
	t.Log("LabelPolicy correctly not ready when org ref missing")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.LabelPolicy{}, 30*time.Second)
}

// --- PrivacyPolicy: org ref not ready ---

func TestPrivacyPolicy_OrgRefNotReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("privpol-noorg-%d", ts)

	cr := &zitadelv1alpha2.PrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.PrivacyPolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: "nonexistent-org-priv"},
			PrivacyPolicyFields: zitadelv1alpha2.PrivacyPolicyFields{
				TosLink: "https://example.com/tos",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating PrivacyPolicy: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.PrivacyPolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting PrivacyPolicy: %v", err)
	}
	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when organizationRef doesn't exist")
	}
	t.Log("PrivacyPolicy correctly not ready when org ref missing")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.PrivacyPolicy{}, 30*time.Second)
}
