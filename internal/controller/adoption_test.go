package controller

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
)

// fakeApplicationClient implements only GenerateClientSecret; all other
// ApplicationServiceClient methods come from the embedded interface and panic
// if called, which is what we want in these tests.
type fakeApplicationClient struct {
	applicationv2.ApplicationServiceClient
	generateCalls int
	lastRequest   *applicationv2.GenerateClientSecretRequest
	secret        string
	err           error
}

func (f *fakeApplicationClient) GenerateClientSecret(_ context.Context, in *applicationv2.GenerateClientSecretRequest, _ ...grpc.CallOption) (*applicationv2.GenerateClientSecretResponse, error) {
	f.generateCalls++
	f.lastRequest = in
	if f.err != nil {
		return nil, f.err
	}
	return &applicationv2.GenerateClientSecretResponse{ClientSecret: f.secret}, nil
}

func newFakeK8s(t *testing.T, objs ...runtime.Object) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...)
}

func TestRegenerateAdoptedClientSecret_SecretMissing(t *testing.T) {
	k8s := newFakeK8s(t).Build()
	apps := &fakeApplicationClient{secret: "fresh-secret"}

	got, err := regenerateAdoptedClientSecret(context.Background(), k8s, apps,
		"argocd", "argocd-oidc", "client_secret", "proj-1", "app-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fresh-secret" {
		t.Fatalf("expected fresh-secret, got %q", got)
	}
	if apps.generateCalls != 1 {
		t.Fatalf("expected 1 GenerateClientSecret call, got %d", apps.generateCalls)
	}
	if apps.lastRequest.GetApplicationId() != "app-1" || apps.lastRequest.GetProjectId() != "proj-1" {
		t.Fatalf("unexpected request: %+v", apps.lastRequest)
	}
}

func TestRegenerateAdoptedClientSecret_KeyEmpty(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "argocd-oidc", Namespace: "argocd"},
		Data: map[string][]byte{
			"client_id":     []byte("client-123"),
			"client_secret": []byte(""),
		},
	}
	k8s := newFakeK8s(t, secret).Build()
	apps := &fakeApplicationClient{secret: "fresh-secret"}

	got, err := regenerateAdoptedClientSecret(context.Background(), k8s, apps,
		"argocd", "argocd-oidc", "client_secret", "proj-1", "app-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fresh-secret" {
		t.Fatalf("expected fresh-secret, got %q", got)
	}
	if apps.generateCalls != 1 {
		t.Fatalf("expected 1 GenerateClientSecret call, got %d", apps.generateCalls)
	}
}

func TestRegenerateAdoptedClientSecret_SecretAlreadyPopulated(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "argocd-oidc", Namespace: "argocd"},
		Data: map[string][]byte{
			"client_id":     []byte("client-123"),
			"client_secret": []byte("existing-secret"),
		},
	}
	k8s := newFakeK8s(t, secret).Build()
	apps := &fakeApplicationClient{secret: "fresh-secret"}

	got, err := regenerateAdoptedClientSecret(context.Background(), k8s, apps,
		"argocd", "argocd-oidc", "client_secret", "proj-1", "app-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected no rotation (empty secret), got %q", got)
	}
	if apps.generateCalls != 0 {
		t.Fatalf("expected no GenerateClientSecret calls, got %d", apps.generateCalls)
	}
}

func TestRegenerateAdoptedClientSecret_CustomKey(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "argocd-oidc", Namespace: "argocd"},
		Data: map[string][]byte{
			// Populated under the default key, but the CR uses a custom key.
			"client_secret": []byte("existing-secret"),
		},
	}
	k8s := newFakeK8s(t, secret).Build()
	apps := &fakeApplicationClient{secret: "fresh-secret"}

	got, err := regenerateAdoptedClientSecret(context.Background(), k8s, apps,
		"argocd", "argocd-oidc", "oidc.clientSecret", "proj-1", "app-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fresh-secret" {
		t.Fatalf("expected fresh-secret for missing custom key, got %q", got)
	}
	if apps.generateCalls != 1 {
		t.Fatalf("expected 1 GenerateClientSecret call, got %d", apps.generateCalls)
	}
}

func TestRegenerateAdoptedClientSecret_GenerateError(t *testing.T) {
	k8s := newFakeK8s(t).Build()
	apps := &fakeApplicationClient{err: errors.New("zitadel unavailable")}

	_, err := regenerateAdoptedClientSecret(context.Background(), k8s, apps,
		"argocd", "argocd-oidc", "client_secret", "proj-1", "app-1")
	if err == nil {
		t.Fatal("expected error when GenerateClientSecret fails")
	}
}

func TestClientSecretKey_Defaults(t *testing.T) {
	cr := &zitadelv1alpha2.OIDCApp{}
	if got := clientSecretKey(cr); got != "client_secret" {
		t.Fatalf("expected default key client_secret, got %q", got)
	}

	cr.Spec.SecretRef.Keys = &zitadelv1alpha2.SecretKeys{ClientSecret: "custom"}
	if got := clientSecretKey(cr); got != "custom" {
		t.Fatalf("expected custom key, got %q", got)
	}
}
