//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

// waitForReady polls until the given resource has Status.Ready=true.
func waitForReady(t *testing.T, ctx context.Context, key client.ObjectKey, obj client.Object, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(ctx, key, obj); err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if isReady(obj) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s/%s to become ready", key.Namespace, key.Name)
}

// waitForDeletion polls until the given resource is gone.
func waitForDeletion(t *testing.T, ctx context.Context, key client.ObjectKey, obj client.Object, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(ctx, key, obj); err != nil {
			if errors.IsNotFound(err) {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s/%s to be deleted", key.Namespace, key.Name)
}

// isReady checks Status.Ready for all v1alpha2 types.
func isReady(obj client.Object) bool {
	switch o := obj.(type) {
	case *zitadelv1alpha2.Organization:
		return o.Status.Ready
	case *zitadelv1alpha2.Project:
		return o.Status.Ready
	case *zitadelv1alpha2.OIDCApp:
		return o.Status.Ready
	case *zitadelv1alpha2.APIApp:
		return o.Status.Ready
	case *zitadelv1alpha2.SAMLApp:
		return o.Status.Ready
	case *zitadelv1alpha2.ApplicationKey:
		return o.Status.Ready
	case *zitadelv1alpha2.PersonalAccessToken:
		return o.Status.Ready
	case *zitadelv1alpha2.MachineUser:
		return o.Status.Ready
	case *zitadelv1alpha2.UserGrant:
		return o.Status.Ready
	case *zitadelv1alpha2.ActionTarget:
		return o.Status.Ready
	case *zitadelv1alpha2.ActionExecution:
		return o.Status.Ready
	case *zitadelv1alpha2.ProjectMember:
		return o.Status.Ready
	case *zitadelv1alpha2.ProjectGrantMember:
		return o.Status.Ready
	case *zitadelv1alpha2.OrgMetadata:
		return o.Status.Ready
	case *zitadelv1alpha2.Domain:
		return o.Status.Ready
	case *zitadelv1alpha2.ProjectGrant:
		return o.Status.Ready
	case *zitadelv1alpha2.IdentityProvider:
		return o.Status.Ready
	case *zitadelv1alpha2.DefaultLoginPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.DefaultDomainPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.GoogleIdP:
		return o.Status.Ready
	case *zitadelv1alpha2.LoginPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.PasswordComplexityPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.LockoutPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.EmailProvider:
		return o.Status.Ready
	case *zitadelv1alpha2.HumanUser:
		return o.Status.Ready
	case *zitadelv1alpha2.OrgMember:
		return o.Status.Ready
	case *zitadelv1alpha2.InstanceMember:
		return o.Status.Ready
	case *zitadelv1alpha2.LabelPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.NotificationPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.PasswordAgePolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.SmsProvider:
		return o.Status.Ready
	case *zitadelv1alpha2.GitHubIdP:
		return o.Status.Ready
	case *zitadelv1alpha2.DefaultLockoutPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.DefaultPasswordComplexityPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.DefaultPasswordAgePolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.DefaultNotificationPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.DefaultLabelPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.DefaultPrivacyPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.DefaultOIDCSettings:
		return o.Status.Ready
	case *zitadelv1alpha2.PrivacyPolicy:
		return o.Status.Ready
	case *zitadelv1alpha2.DefaultMessageText:
		return o.Status.Ready
	case *zitadelv1alpha2.MessageText:
		return o.Status.Ready
	default:
		return false
	}
}
