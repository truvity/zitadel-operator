//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

// --- P0: Instance-Level Resources (IAM_OWNER, Admin API) ---

func TestDefaultLoginPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("deflogin-%d", time.Now().UnixMilli())

	allowExternal := true
	forceMfa := false
	cr := &zitadelv1alpha2.DefaultLoginPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultLoginPolicySpec{
			AllowExternalIdp: &allowExternal,
			ForceMfa:         &forceMfa,
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultLoginPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultLoginPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("DefaultLoginPolicy reconciled: ready=%v", reconciled.Status.Ready)

	// Mutate: disable external idp.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting DefaultLoginPolicy: %v", err)
	}
	disallow := false
	cr.Spec.AllowExternalIdp = &disallow
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating DefaultLoginPolicy: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting DefaultLoginPolicy after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}
	t.Log("DefaultLoginPolicy mutation reconciled successfully")

	// Cleanup (resets to safe defaults via finalizer).
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultLoginPolicy{}, 30*time.Second)
	t.Log("DefaultLoginPolicy lifecycle complete")
}

func TestDefaultDomainPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("defdomain-%d", time.Now().UnixMilli())

	userLoginMust := false
	cr := &zitadelv1alpha2.DefaultDomainPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultDomainPolicySpec{
			UserLoginMustBeDomain: &userLoginMust,
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultDomainPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.DefaultDomainPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("DefaultDomainPolicy reconciled: ready=%v", reconciled.Status.Ready)

	// Mutate.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting DefaultDomainPolicy: %v", err)
	}
	validate := true
	cr.Spec.ValidateOrgDomains = &validate
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating DefaultDomainPolicy: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting DefaultDomainPolicy after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultDomainPolicy{}, 30*time.Second)
	t.Log("DefaultDomainPolicy lifecycle complete")
}

func TestGoogleIdP_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("googleidp-%d", ts)
	secretName := fmt.Sprintf("googleidp-secret-%d", ts)

	// Create secret with client secret.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"},
		Data: map[string][]byte{
			"clientSecret": []byte("test-client-secret-" + fmt.Sprint(ts)),
		},
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("creating secret: %v", err)
	}

	cr := &zitadelv1alpha2.GoogleIdP{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.GoogleIdPSpec{
			Name:     fmt.Sprintf("Test Google IdP %d", ts),
			ClientId: fmt.Sprintf("test-client-id-%d.apps.googleusercontent.com", ts),
			ClientSecretRef: zitadelv1alpha2.SecretKeyRef{
				Name: secretName,
				Key:  "clientSecret",
			},
			Scopes:           []string{"openid", "email", "profile"},
			IsLinkingAllowed: true,
			IsAutoCreation:   true,
			IsAutoUpdate:     true,
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating GoogleIdP: %v", err)
	}

	var reconciled zitadelv1alpha2.GoogleIdP
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("GoogleIdP reconciled: idpID=%s, ready=%v", reconciled.Status.IdpID, reconciled.Status.Ready)

	if reconciled.Status.IdpID == "" {
		t.Fatal("expected idpID to be set")
	}

	// Mutate: update scopes.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting GoogleIdP: %v", err)
	}
	cr.Spec.Scopes = []string{"openid", "email"}
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating GoogleIdP: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting GoogleIdP after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.GoogleIdP{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, secret)
	t.Log("GoogleIdP lifecycle complete")
}

// --- P1: Org-Scoped Policies (ORG_OWNER, Management API) ---

func TestLoginPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("loginpol-org-%d", ts)
	policyName := fmt.Sprintf("loginpol-%d", ts)

	// Create org first.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create login policy.
	allowReg := true
	cr := &zitadelv1alpha2.LoginPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: zitadelv1alpha2.LoginPolicySpec{
			OrganizationRef:   &zitadelv1alpha2.ResourceRef{Name: orgName},
			LoginPolicyFields: zitadelv1alpha2.LoginPolicyFields{AllowRegister: &allowReg},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating LoginPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.LoginPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("LoginPolicy reconciled: orgId=%s, ready=%v", reconciled.Status.OrganizationId, reconciled.Status.Ready)

	// Mutate.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting LoginPolicy: %v", err)
	}
	disallow := false
	cr.Spec.LoginPolicyFields.AllowRegister = &disallow
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating LoginPolicy: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting LoginPolicy after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.LoginPolicy{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	t.Log("LoginPolicy lifecycle complete")
}

func TestPasswordComplexityPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("pwcplx-org-%d", ts)
	policyName := fmt.Sprintf("pwcplx-%d", ts)

	// Create org.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create password complexity policy.
	cr := &zitadelv1alpha2.PasswordComplexityPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: zitadelv1alpha2.PasswordComplexityPolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			PasswordComplexityPolicyFields: zitadelv1alpha2.PasswordComplexityPolicyFields{
				MinLength:    12,
				HasLowercase: true,
				HasUppercase: true,
				HasNumber:    true,
				HasSymbol:    false,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating PasswordComplexityPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.PasswordComplexityPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("PasswordComplexityPolicy reconciled: orgId=%s, ready=%v", reconciled.Status.OrganizationId, reconciled.Status.Ready)

	// Mutate: change minLength.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting PasswordComplexityPolicy: %v", err)
	}
	cr.Spec.PasswordComplexityPolicyFields.MinLength = 16
	cr.Spec.PasswordComplexityPolicyFields.HasSymbol = true
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating PasswordComplexityPolicy: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting PasswordComplexityPolicy after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.PasswordComplexityPolicy{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	t.Log("PasswordComplexityPolicy lifecycle complete")
}

func TestLockoutPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("lockout-org-%d", ts)
	policyName := fmt.Sprintf("lockout-%d", ts)

	// Create org.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create lockout policy.
	cr := &zitadelv1alpha2.LockoutPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: zitadelv1alpha2.LockoutPolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			LockoutPolicyFields: zitadelv1alpha2.LockoutPolicyFields{
				MaxPasswordAttempts: 5,
				MaxOtpAttempts:      3,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating LockoutPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.LockoutPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("LockoutPolicy reconciled: orgId=%s, ready=%v", reconciled.Status.OrganizationId, reconciled.Status.Ready)

	// Mutate.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting LockoutPolicy: %v", err)
	}
	cr.Spec.LockoutPolicyFields.MaxPasswordAttempts = 10
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating LockoutPolicy: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting LockoutPolicy after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.LockoutPolicy{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	t.Log("LockoutPolicy lifecycle complete")
}

// --- P2: Notification (IAM_OWNER, Admin API) ---

func TestEmailProvider_HTTP_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("emailhttp-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.EmailProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.EmailProviderSpec{
			Http: &zitadelv1alpha2.HttpEmailProvider{
				Endpoint: "https://httpbin.org/post",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating EmailProvider (HTTP): %v", err)
	}

	var reconciled zitadelv1alpha2.EmailProvider
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("EmailProvider (HTTP) reconciled: providerId=%s, ready=%v", reconciled.Status.ProviderId, reconciled.Status.Ready)

	if reconciled.Status.ProviderId == "" {
		t.Fatal("expected providerId to be set")
	}

	// Mutate endpoint.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting EmailProvider: %v", err)
	}
	cr.Spec.Http.Endpoint = "https://httpbin.org/anything"
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating EmailProvider: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting EmailProvider after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.EmailProvider{}, 30*time.Second)
	t.Log("EmailProvider (HTTP) lifecycle complete")
}

func TestEmailProvider_SMTP_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("emailsmtp-%d", ts)
	secretName := fmt.Sprintf("smtp-pw-%d", ts)

	// Use the instance domain as sender address (Zitadel Cloud requires sender domain
	// to be a registered Custom Domain on the instance).
	senderDomain := cfg.Domain

	// Create password secret.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"},
		Data: map[string][]byte{
			"password": []byte("test-smtp-password"),
		},
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("creating secret: %v", err)
	}

	cr := &zitadelv1alpha2.EmailProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.EmailProviderSpec{
			Smtp: &zitadelv1alpha2.SmtpEmailProvider{
				Description:   "Test SMTP",
				SenderAddress: fmt.Sprintf("noreply-%d@%s", ts, senderDomain),
				SenderName:    "Test Sender",
				Host:          "smtp.example.com:587",
				Tls:           true,
				User:          "smtp-user",
				PasswordSecretRef: &zitadelv1alpha2.SecretKeyRef{
					Name: secretName,
					Key:  "password",
				},
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating EmailProvider (SMTP): %v", err)
	}

	var reconciled zitadelv1alpha2.EmailProvider
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("EmailProvider (SMTP) reconciled: providerId=%s, ready=%v", reconciled.Status.ProviderId, reconciled.Status.Ready)

	if reconciled.Status.ProviderId == "" {
		t.Fatal("expected providerId to be set")
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.EmailProvider{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, secret)
	t.Log("EmailProvider (SMTP) lifecycle complete")
}

// --- Negative Cases ---

func TestGoogleIdP_SecretNotFound(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("googleidp-nosecret-%d", ts)

	cr := &zitadelv1alpha2.GoogleIdP{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.GoogleIdPSpec{
			Name:     fmt.Sprintf("NoSecret Google %d", ts),
			ClientId: fmt.Sprintf("nosecret-%d.apps.googleusercontent.com", ts),
			ClientSecretRef: zitadelv1alpha2.SecretKeyRef{
				Name: "nonexistent-secret",
				Key:  "clientSecret",
			},
			Scopes:         []string{"openid"},
			IsAutoCreation: true,
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating GoogleIdP: %v", err)
	}

	// Wait for reconciliation attempt.
	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.GoogleIdP
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting GoogleIdP: %v", err)
	}

	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when secret doesn't exist")
	}

	// Check condition.
	readyCond := findCondition(reconciled.Status.Conditions, "Ready")
	if readyCond == nil {
		t.Fatal("expected Ready condition to be set")
	}
	if readyCond.Status != metav1.ConditionFalse {
		t.Errorf("expected Ready=False, got %s", readyCond.Status)
	}
	if readyCond.Reason != "SecretNotFound" {
		t.Errorf("expected reason=SecretNotFound, got %s", readyCond.Reason)
	}
	t.Logf("GoogleIdP correctly not ready: reason=%s, message=%s", readyCond.Reason, readyCond.Message)

	// Now create the secret — should resolve.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "nonexistent-secret", Namespace: "default"},
		Data: map[string][]byte{
			"clientSecret": []byte("now-it-exists-" + fmt.Sprint(ts)),
		},
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("creating secret: %v", err)
	}

	// Wait for resolution.
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after secret creation")
	}
	if reconciled.Status.IdpID == "" {
		t.Fatal("expected idpID to be set after resolution")
	}
	t.Logf("GoogleIdP resolved after secret creation: idpID=%s", reconciled.Status.IdpID)

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.GoogleIdP{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, secret)
	t.Log("GoogleIdP secret-not-found negative case complete")
}

func TestDefaultLoginPolicy_IdpRefNotReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	policyName := fmt.Sprintf("deflogin-noref-%d", ts)

	// Create policy referencing a GoogleIdP that doesn't exist yet.
	allowExternal := true
	cr := &zitadelv1alpha2.DefaultLoginPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultLoginPolicySpec{
			AllowExternalIdp: &allowExternal,
			Idps: []zitadelv1alpha2.IdpReference{
				{IdpRef: &zitadelv1alpha2.ResourceRef{Name: "nonexistent-idp"}},
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating DefaultLoginPolicy: %v", err)
	}

	// Wait for reconcile attempt.
	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.DefaultLoginPolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting DefaultLoginPolicy: %v", err)
	}

	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when idpRef doesn't exist")
	}

	readyCond := findCondition(reconciled.Status.Conditions, "Ready")
	if readyCond == nil {
		t.Fatal("expected Ready condition to be set")
	}
	if readyCond.Status != metav1.ConditionFalse {
		t.Errorf("expected Ready=False, got %s", readyCond.Status)
	}
	if readyCond.Reason != "IdpNotReady" {
		t.Errorf("expected reason=IdpNotReady, got %s", readyCond.Reason)
	}
	t.Logf("DefaultLoginPolicy correctly not ready: reason=%s", readyCond.Reason)

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.DefaultLoginPolicy{}, 30*time.Second)
	t.Log("DefaultLoginPolicy idpRef-not-ready negative case complete")
}

func TestLoginPolicy_OrgRefNotReady(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	policyName := fmt.Sprintf("loginpol-noorg-%d", ts)

	// Create login policy referencing an org that doesn't exist.
	allowReg := true
	cr := &zitadelv1alpha2.LoginPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: zitadelv1alpha2.LoginPolicySpec{
			OrganizationRef:   &zitadelv1alpha2.ResourceRef{Name: "nonexistent-org"},
			LoginPolicyFields: zitadelv1alpha2.LoginPolicyFields{AllowRegister: &allowReg},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating LoginPolicy: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.LoginPolicy
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting LoginPolicy: %v", err)
	}

	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when organizationRef doesn't exist")
	}

	readyCond := findCondition(reconciled.Status.Conditions, "Ready")
	if readyCond != nil {
		if readyCond.Status != metav1.ConditionFalse {
			t.Errorf("expected Ready=False, got %s", readyCond.Status)
		}
		if readyCond.Reason != "OrgNotReady" {
			t.Errorf("expected reason=OrgNotReady, got %s", readyCond.Reason)
		}
		t.Logf("LoginPolicy correctly not ready: reason=%s", readyCond.Reason)
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.LoginPolicy{}, 30*time.Second)
	t.Log("LoginPolicy org-ref-not-ready negative case complete")
}

func TestEmailProvider_InvalidSpec_BothSmtpAndHttp(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("email-invalid-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.EmailProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.EmailProviderSpec{
			Smtp: &zitadelv1alpha2.SmtpEmailProvider{
				SenderAddress: "test@example.com",
				Host:          "smtp.example.com:587",
			},
			Http: &zitadelv1alpha2.HttpEmailProvider{
				Endpoint: "https://httpbin.org/post",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating EmailProvider: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.EmailProvider
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting EmailProvider: %v", err)
	}

	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when both smtp and http are set")
	}

	readyCond := findCondition(reconciled.Status.Conditions, "Ready")
	if readyCond == nil {
		t.Fatal("expected Ready condition to be set")
	}
	if readyCond.Reason != "InvalidSpec" {
		t.Errorf("expected reason=InvalidSpec, got %s", readyCond.Reason)
	}
	t.Logf("EmailProvider correctly rejected: reason=%s, message=%s", readyCond.Reason, readyCond.Message)

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.EmailProvider{}, 30*time.Second)
	t.Log("EmailProvider invalid-spec negative case complete")
}

func TestEmailProvider_SmtpPasswordSecretNotFound(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("email-nopw-%d", ts)

	cr := &zitadelv1alpha2.EmailProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.EmailProviderSpec{
			Smtp: &zitadelv1alpha2.SmtpEmailProvider{
				SenderAddress: fmt.Sprintf("nopw-%d@example.com", ts),
				Host:          "smtp.example.com:587",
				PasswordSecretRef: &zitadelv1alpha2.SecretKeyRef{
					Name: "nonexistent-smtp-secret",
					Key:  "password",
				},
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating EmailProvider: %v", err)
	}

	time.Sleep(5 * time.Second)

	var reconciled zitadelv1alpha2.EmailProvider
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting EmailProvider: %v", err)
	}

	if reconciled.Status.Ready {
		t.Fatal("expected Ready=false when password secret doesn't exist")
	}

	readyCond := findCondition(reconciled.Status.Conditions, "Ready")
	if readyCond != nil {
		if readyCond.Status != metav1.ConditionFalse {
			t.Errorf("expected Ready=False, got %s", readyCond.Status)
		}
		t.Logf("EmailProvider correctly not ready: reason=%s", readyCond.Reason)
	}

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.EmailProvider{}, 30*time.Second)
	t.Log("EmailProvider smtp-password-not-found negative case complete")
}

// --- GoogleIdP → DefaultLoginPolicy idpRef Resolution ---

func TestGoogleIdP_DefaultLoginPolicy_IdpRef_Resolution(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	idpName := fmt.Sprintf("idpref-google-%d", ts)
	policyName := fmt.Sprintf("idpref-policy-%d", ts)
	secretName := fmt.Sprintf("idpref-secret-%d", ts)

	// Create client secret.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"},
		Data: map[string][]byte{
			"clientSecret": []byte("resolution-test-secret-" + fmt.Sprint(ts)),
		},
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("creating secret: %v", err)
	}

	// Create GoogleIdP.
	idp := &zitadelv1alpha2.GoogleIdP{
		ObjectMeta: metav1.ObjectMeta{Name: idpName, Namespace: "default"},
		Spec: zitadelv1alpha2.GoogleIdPSpec{
			Name:     fmt.Sprintf("Resolution Test Google %d", ts),
			ClientId: fmt.Sprintf("resolve-test-%d.apps.googleusercontent.com", ts),
			ClientSecretRef: zitadelv1alpha2.SecretKeyRef{
				Name: secretName,
				Key:  "clientSecret",
			},
			Scopes:           []string{"openid", "email"},
			IsLinkingAllowed: true,
			IsAutoCreation:   true,
			IsAutoUpdate:     true,
		},
	}
	if err := k8sClient.Create(ctx, idp); err != nil {
		t.Fatalf("creating GoogleIdP: %v", err)
	}

	var reconciledIdp zitadelv1alpha2.GoogleIdP
	waitForReady(t, ctx, client.ObjectKeyFromObject(idp), &reconciledIdp, 30*time.Second)
	t.Logf("GoogleIdP ready: idpID=%s", reconciledIdp.Status.IdpID)

	// Create DefaultLoginPolicy referencing the GoogleIdP via idpRef.
	allowExternal := true
	policy := &zitadelv1alpha2.DefaultLoginPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: zitadelv1alpha2.DefaultLoginPolicySpec{
			AllowExternalIdp: &allowExternal,
			Idps: []zitadelv1alpha2.IdpReference{
				{IdpRef: &zitadelv1alpha2.ResourceRef{Name: idpName}},
			},
		},
	}
	if err := k8sClient.Create(ctx, policy); err != nil {
		t.Fatalf("creating DefaultLoginPolicy with idpRef: %v", err)
	}

	var reconciledPolicy zitadelv1alpha2.DefaultLoginPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(policy), &reconciledPolicy, 30*time.Second)
	t.Logf("DefaultLoginPolicy with idpRef reconciled: ready=%v", reconciledPolicy.Status.Ready)

	// Cleanup (policy first, then idp).
	_ = k8sClient.Delete(ctx, policy)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(policy), &zitadelv1alpha2.DefaultLoginPolicy{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, idp)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(idp), &zitadelv1alpha2.GoogleIdP{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, secret)
	t.Log("GoogleIdP → DefaultLoginPolicy idpRef resolution complete")
}
