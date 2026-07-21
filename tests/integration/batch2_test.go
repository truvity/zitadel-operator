//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zalando/go-keyring"

	corev1 "k8s.io/api/core/v1"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

// --- HumanUser ---

func TestHumanUser_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("hu-org-%d", ts)
	userName := fmt.Sprintf("hu-user-%d", ts)
	crName := fmt.Sprintf("hu-%d", ts)

	// Create org.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create HumanUser.
	cr := &zitadelv1alpha2.HumanUser{
		ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: "default"},
		Spec: zitadelv1alpha2.HumanUserSpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			UserName:        userName,
			FirstName:       "Test",
			LastName:        "User",
			Email:           fmt.Sprintf("test-%d@example.com", ts),
			IsEmailVerified: true,
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating HumanUser: %v", err)
	}

	var reconciled zitadelv1alpha2.HumanUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("HumanUser reconciled: userId=%s, ready=%v", reconciled.Status.UserId, reconciled.Status.Ready)

	if reconciled.Status.UserId == "" {
		t.Fatal("expected userId to be set")
	}

	// Mutate: update DisplayName.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	cr.Spec.FirstName = "Updated"
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
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.HumanUser{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	t.Log("HumanUser lifecycle complete")
}

// --- OrgMember ---

func TestOrgMember_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("om-org-%d", ts)
	muName := fmt.Sprintf("om-mu-%d", ts)
	memberName := fmt.Sprintf("om-%d", ts)

	// Create org.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create MachineUser in that org.
	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{Name: muName, Namespace: "default"},
		Spec: zitadelv1alpha2.MachineUserSpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			UserName:        fmt.Sprintf("orgmember-bot-%d", ts),
			KeySecretRef:    zitadelv1alpha2.MachineKeySecretRef{Name: fmt.Sprintf("om-key-%d", ts)},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser: %v", err)
	}
	var reconciledMu zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciledMu, 30*time.Second)

	// Create OrgMember.
	cr := &zitadelv1alpha2.OrgMember{
		ObjectMeta: metav1.ObjectMeta{Name: memberName, Namespace: "default"},
		Spec: zitadelv1alpha2.OrgMemberSpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			UserRef:         &zitadelv1alpha2.ResourceRef{Name: muName},
			Roles:           []string{"ORG_OWNER"},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating OrgMember: %v", err)
	}

	var reconciled zitadelv1alpha2.OrgMember
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("OrgMember reconciled: ready=%v", reconciled.Status.Ready)

	// Mutate: update Roles.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	cr.Spec.Roles = []string{"ORG_OWNER", "ORG_ADMIN"}
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
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.OrgMember{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, mu)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(mu), &zitadelv1alpha2.MachineUser{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	t.Log("OrgMember lifecycle complete")
}

// --- InstanceMember ---

func TestInstanceMember_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	muName := fmt.Sprintf("im-mu-%d", ts)
	memberName := fmt.Sprintf("im-%d", ts)

	// Create MachineUser (in default org).
	mu := &zitadelv1alpha2.MachineUser{
		ObjectMeta: metav1.ObjectMeta{Name: muName, Namespace: "default"},
		Spec: zitadelv1alpha2.MachineUserSpec{
			OrganizationId: testOrgID,
			UserName:       fmt.Sprintf("instmember-bot-%d", ts),
			KeySecretRef:   zitadelv1alpha2.MachineKeySecretRef{Name: fmt.Sprintf("im-key-%d", ts)},
		},
	}
	if err := k8sClient.Create(ctx, mu); err != nil {
		t.Fatalf("creating MachineUser: %v", err)
	}
	var reconciledMu zitadelv1alpha2.MachineUser
	waitForReady(t, ctx, client.ObjectKeyFromObject(mu), &reconciledMu, 30*time.Second)

	// Create InstanceMember.
	cr := &zitadelv1alpha2.InstanceMember{
		ObjectMeta: metav1.ObjectMeta{Name: memberName, Namespace: "default"},
		Spec: zitadelv1alpha2.InstanceMemberSpec{
			UserRef: &zitadelv1alpha2.ResourceRef{Name: muName},
			Roles:   []string{"IAM_ORG_MANAGER"},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating InstanceMember: %v", err)
	}

	var reconciled zitadelv1alpha2.InstanceMember
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("InstanceMember reconciled: ready=%v", reconciled.Status.Ready)

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.InstanceMember{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, mu)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(mu), &zitadelv1alpha2.MachineUser{}, 30*time.Second)
	t.Log("InstanceMember lifecycle complete")
}

// --- LabelPolicy ---

func TestLabelPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("lp-org-%d", ts)
	policyName := fmt.Sprintf("lp-%d", ts)

	// Create org.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create LabelPolicy.
	cr := &zitadelv1alpha2.LabelPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: zitadelv1alpha2.LabelPolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			LabelPolicyFields: zitadelv1alpha2.LabelPolicyFields{
				PrimaryColor:    "#5469d4",
				BackgroundColor: "#ffffff",
				WarnColor:       "#cd3d56",
				FontColor:       "#000000",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating LabelPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.LabelPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("LabelPolicy reconciled: orgId=%s, ready=%v", reconciled.Status.OrganizationId, reconciled.Status.Ready)

	// Mutate: update PrimaryColor.
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
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.LabelPolicy{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	t.Log("LabelPolicy lifecycle complete")
}

// --- NotificationPolicy ---

func TestNotificationPolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("np-org-%d", ts)
	policyName := fmt.Sprintf("np-%d", ts)

	// Create org.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create NotificationPolicy.
	passwordChange := true
	cr := &zitadelv1alpha2.NotificationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: zitadelv1alpha2.NotificationPolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			NotificationPolicyFields: zitadelv1alpha2.NotificationPolicyFields{
				PasswordChange: &passwordChange,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating NotificationPolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.NotificationPolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("NotificationPolicy reconciled: orgId=%s, ready=%v", reconciled.Status.OrganizationId, reconciled.Status.Ready)

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
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.NotificationPolicy{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	t.Log("NotificationPolicy lifecycle complete")
}

// --- PasswordAgePolicy ---

func TestPasswordAgePolicy_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	orgName := fmt.Sprintf("pap-org-%d", ts)
	policyName := fmt.Sprintf("pap-%d", ts)

	// Create org.
	org := &zitadelv1alpha2.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: orgName, Namespace: "default"},
	}
	if err := k8sClient.Create(ctx, org); err != nil {
		t.Fatalf("creating Organization: %v", err)
	}
	var reconciledOrg zitadelv1alpha2.Organization
	waitForReady(t, ctx, client.ObjectKeyFromObject(org), &reconciledOrg, 30*time.Second)

	// Create PasswordAgePolicy.
	cr := &zitadelv1alpha2.PasswordAgePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: zitadelv1alpha2.PasswordAgePolicySpec{
			OrganizationRef: &zitadelv1alpha2.ResourceRef{Name: orgName},
			PasswordAgePolicyFields: zitadelv1alpha2.PasswordAgePolicyFields{
				MaxAgeDays:     90,
				ExpireWarnDays: 14,
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating PasswordAgePolicy: %v", err)
	}

	var reconciled zitadelv1alpha2.PasswordAgePolicy
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("PasswordAgePolicy reconciled: orgId=%s, ready=%v", reconciled.Status.OrganizationId, reconciled.Status.Ready)

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
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.PasswordAgePolicy{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, org)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(org), &zitadelv1alpha2.Organization{}, 30*time.Second)
	t.Log("PasswordAgePolicy lifecycle complete")
}

// --- SmsProvider (HTTP) ---

func TestSmsProvider_HTTP_Lifecycle(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("smshttp-%d", time.Now().UnixMilli())

	cr := &zitadelv1alpha2.SmsProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.SmsProviderSpec{
			Http: &zitadelv1alpha2.HttpSmsProvider{
				Endpoint:    "https://httpbin.org/post",
				Description: "Test HTTP SMS",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating SmsProvider (HTTP): %v", err)
	}

	var reconciled zitadelv1alpha2.SmsProvider
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("SmsProvider (HTTP) reconciled: providerId=%s, ready=%v", reconciled.Status.ProviderId, reconciled.Status.Ready)

	if reconciled.Status.ProviderId == "" {
		t.Fatal("expected providerId to be set")
	}

	// Mutate: change Endpoint.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting: %v", err)
	}
	cr.Spec.Http.Endpoint = "https://httpbin.org/anything"
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
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.SmsProvider{}, 30*time.Second)
	t.Log("SmsProvider (HTTP) lifecycle complete")
}

// --- GitHubIdP ---

func TestGitHubIdP_Lifecycle(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	name := fmt.Sprintf("githubidp-%d", ts)
	secretName := fmt.Sprintf("github-secret-%d", ts)

	// Load GitHub OAuth App credentials from keyring.
	clientID, err := keyring.Get("zitadel-operator", "github-idp-client-id")
	if err != nil {
		t.Skip("Skipping GitHubIdP test: github-idp-client-id not found in keyring. " +
			"Store with: echo -n 'CLIENT_ID' | secret-tool store --label='zitadel-operator github-idp-client-id' service zitadel-operator username github-idp-client-id")
	}
	clientSecret, err := keyring.Get("zitadel-operator", "github-idp-secret")
	if err != nil {
		t.Skip("Skipping GitHubIdP test: github-idp-secret not found in keyring. " +
			"Store with: echo -n 'SECRET' | secret-tool store --label='zitadel-operator github-idp-secret' service zitadel-operator username github-idp-secret")
	}

	// Create K8s secret with the GitHub client secret.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"},
		Data: map[string][]byte{
			"clientSecret": []byte(clientSecret),
		},
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("creating secret: %v", err)
	}

	// Create GitHubIdP.
	cr := &zitadelv1alpha2.GitHubIdP{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zitadelv1alpha2.GitHubIdPSpec{
			Name:     fmt.Sprintf("Test GitHub IdP %d", ts),
			ClientId: clientID,
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
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("creating GitHubIdP: %v", err)
	}

	var reconciled zitadelv1alpha2.GitHubIdP
	waitForReady(t, ctx, client.ObjectKeyFromObject(cr), &reconciled, 30*time.Second)
	t.Logf("GitHubIdP reconciled: idpID=%s, ready=%v", reconciled.Status.IdpID, reconciled.Status.Ready)

	if reconciled.Status.IdpID == "" {
		t.Fatal("expected idpID to be set")
	}

	// Mutate: update scopes.
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatalf("getting GitHubIdP: %v", err)
	}
	cr.Spec.Scopes = []string{"openid", "email", "read:user"}
	if err := k8sClient.Update(ctx, cr); err != nil {
		t.Fatalf("updating GitHubIdP: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &reconciled); err != nil {
		t.Fatalf("getting GitHubIdP after update: %v", err)
	}
	if !reconciled.Status.Ready {
		t.Fatal("expected Ready=true after mutation")
	}
	t.Log("GitHubIdP mutation reconciled")

	// Cleanup.
	_ = k8sClient.Delete(ctx, cr)
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(cr), &zitadelv1alpha2.GitHubIdP{}, 30*time.Second)
	_ = k8sClient.Delete(ctx, secret)
	t.Log("GitHubIdP lifecycle complete")
}
