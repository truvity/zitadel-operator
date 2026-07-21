package scopemap

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

const (
	testInstance = "zitadel.test.example"
	operatorNS   = "zitadel-operator-system"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := zitadelv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func namespace(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func scopeMap(name string, rules ...zitadelv1alpha2.ScopeMapRule) *zitadelv1alpha2.ZitadelScopeMap {
	return &zitadelv1alpha2.ZitadelScopeMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: operatorNS},
		Spec: zitadelv1alpha2.ZitadelScopeMapSpec{
			Instance:       testInstance,
			Organization:   "Test Org",
			OrganizationId: "123456",
			Rules:          rules,
		},
	}
}

func newResolver(t *testing.T, synced bool, objs ...runtime.Object) *Resolver {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).WithRuntimeObjects(objs...).Build()
	return &Resolver{
		Reader:    c,
		Namespace: operatorNS,
		Instance:  testInstance,
		Synced:    func() bool { return synced },
		Recorder:  record.NewFakeRecorder(16),
	}
}

func TestResolve_MapsNotSynced(t *testing.T) {
	r := newResolver(t, false)
	_, err := r.Resolve(context.Background(), "tenant-a")
	if !errors.Is(err, ErrMapsNotSynced) {
		t.Fatalf("expected ErrMapsNotSynced, got %v", err)
	}
}

func TestResolve_NoMaps_Passthrough(t *testing.T) {
	r := newResolver(t, true, namespace("tenant-a", nil))
	s, err := r.Resolve(context.Background(), "tenant-a")
	if err != nil || s != nil {
		t.Fatalf("expected passthrough (nil, nil), got scope=%v err=%v", s, err)
	}
}

func TestResolve_SelectorMatch(t *testing.T) {
	m := scopeMap("map-a", zitadelv1alpha2.ScopeMapRule{
		Name: "by-team",
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"team": "alpha"},
		},
		Project: "alpha-project",
	})
	r := newResolver(t, true, m, namespace("tenant-a", map[string]string{"team": "alpha"}))
	s, err := r.Resolve(context.Background(), "tenant-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ProjectName != "alpha-project" || s.OrganizationID != "123456" || s.RuleName != "by-team" {
		t.Fatalf("unexpected scope: %+v", s)
	}
}

func TestResolve_LiteralMatch_FirstMatchTopDown(t *testing.T) {
	m := scopeMap("map-a",
		zitadelv1alpha2.ScopeMapRule{Name: "first", Namespaces: []string{"tenant-b"}, Project: "proj-first"},
		zitadelv1alpha2.ScopeMapRule{Name: "second", Namespaces: []string{"tenant-b"}, Project: "proj-second"},
	)
	r := newResolver(t, true, m, namespace("tenant-b", nil))
	s, err := r.Resolve(context.Background(), "tenant-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.RuleName != "first" || s.ProjectName != "proj-first" {
		t.Fatalf("expected first rule to win, got %+v", s)
	}
}

func TestResolve_NoMatch_FailClosed(t *testing.T) {
	m := scopeMap("map-a", zitadelv1alpha2.ScopeMapRule{Name: "r", Namespaces: []string{"other"}})
	r := newResolver(t, true, m, namespace("tenant-c", nil))
	_, err := r.Resolve(context.Background(), "tenant-c")
	var noMatch *NoMatchError
	if !errors.As(err, &noMatch) {
		t.Fatalf("expected NoMatchError, got %v", err)
	}
}

func TestResolve_CrossMapConflict(t *testing.T) {
	m1 := scopeMap("map-a", zitadelv1alpha2.ScopeMapRule{Name: "r1", Namespaces: []string{"tenant-d"}})
	m2 := scopeMap("map-b", zitadelv1alpha2.ScopeMapRule{Name: "r2", Namespaces: []string{"tenant-d"}})
	rec := record.NewFakeRecorder(16)
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).
		WithRuntimeObjects(m1, m2, namespace("tenant-d", nil)).Build()
	r := &Resolver{Reader: c, Namespace: operatorNS, Instance: testInstance,
		Synced: func() bool { return true }, Recorder: rec}

	_, err := r.Resolve(context.Background(), "tenant-d")
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected ConflictError, got %v", err)
	}
	if len(conflict.Maps) != 2 {
		t.Fatalf("expected 2 conflicting maps, got %v", conflict.Maps)
	}
	// Events on both maps.
	if got := len(rec.Events); got != 2 {
		t.Fatalf("expected 2 conflict events, got %d", got)
	}
}

func TestResolve_InstanceMismatch(t *testing.T) {
	m := scopeMap("map-a", zitadelv1alpha2.ScopeMapRule{Name: "r", Namespaces: []string{"tenant-e"}})
	m.Spec.Instance = "other.instance.example"
	r := newResolver(t, true, m, namespace("tenant-e", nil))
	_, err := r.Resolve(context.Background(), "tenant-e")
	var mismatch *InstanceMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected InstanceMismatchError, got %v", err)
	}
}

func TestResolve_MapNotReady_NoOrgID(t *testing.T) {
	m := scopeMap("map-a", zitadelv1alpha2.ScopeMapRule{Name: "r", Namespaces: []string{"tenant-f"}})
	m.Spec.OrganizationId = "" // requires status.resolvedOrganizationId, absent
	r := newResolver(t, true, m, namespace("tenant-f", nil))
	_, err := r.Resolve(context.Background(), "tenant-f")
	var notReady *MapNotReadyError
	if !errors.As(err, &notReady) {
		t.Fatalf("expected MapNotReadyError, got %v", err)
	}
}

func TestValidateRule(t *testing.T) {
	sel := &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}}
	cases := []struct {
		name    string
		rule    zitadelv1alpha2.ScopeMapRule
		wantErr bool
	}{
		{"selector only", zitadelv1alpha2.ScopeMapRule{Name: "r", NamespaceSelector: sel}, false},
		{"literal only", zitadelv1alpha2.ScopeMapRule{Name: "r", Namespaces: []string{"x"}}, false},
		{"both", zitadelv1alpha2.ScopeMapRule{Name: "r", NamespaceSelector: sel, Namespaces: []string{"x"}}, true},
		{"neither", zitadelv1alpha2.ScopeMapRule{Name: "r"}, true},
		{"projectId without project", zitadelv1alpha2.ScopeMapRule{Name: "r", Namespaces: []string{"x"}, ProjectId: "42"}, true},
		{"projectId with project", zitadelv1alpha2.ScopeMapRule{Name: "r", Namespaces: []string{"x"}, Project: "p", ProjectId: "42"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRule(&tc.rule)
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v got %v", tc.wantErr, err)
			}
		})
	}
}

func TestScopeHash_StableAndDistinct(t *testing.T) {
	s1 := &Scope{Instance: testInstance, OrganizationID: "1", ProjectName: "p"}
	s2 := &Scope{Instance: testInstance, OrganizationID: "1", ProjectName: "p"}
	s3 := &Scope{Instance: testInstance, OrganizationID: "1"}
	if s1.Hash() != s2.Hash() {
		t.Fatal("identical scopes must hash equal")
	}
	if s1.Hash() == s3.Hash() {
		t.Fatal("org-scope and project-scope must hash differently")
	}
	if len(s1.Hash()) != 10 {
		t.Fatalf("hash length: %d", len(s1.Hash()))
	}
}
