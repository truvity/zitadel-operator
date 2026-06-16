package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/truvity/zitadel-operator/internal/config"
)

func TestValidateProjectScope_EmptyLabel(t *testing.T) {
	cfg := &config.Config{
		ProjectScopeLabel: "",
	}

	// No client needed — function should short-circuit.
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()

	var conditions []metav1.Condition
	shouldProceed, err := validateProjectScope(context.Background(), k8s, cfg, "test-ns", &conditions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !shouldProceed {
		t.Fatal("expected shouldProceed=true when projectScopeLabel is empty")
	}
	if len(conditions) != 0 {
		t.Fatalf("expected no conditions, got %d", len(conditions))
	}
}

func TestValidateProjectScope_LabelPresent(t *testing.T) {
	cfg := &config.Config{
		ProjectScopeLabel: "zitadel.truvity.com/employee-project",
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "argocd",
			Labels: map[string]string{
				"zitadel.truvity.com/employee-project": "cluster-devel",
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

	var conditions []metav1.Condition
	shouldProceed, err := validateProjectScope(context.Background(), k8s, cfg, "argocd", &conditions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !shouldProceed {
		t.Fatal("expected shouldProceed=true when label is present")
	}
	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conditions))
	}
	if conditions[0].Type != ConditionTypeProjectScope {
		t.Fatalf("expected condition type %q, got %q", ConditionTypeProjectScope, conditions[0].Type)
	}
	if conditions[0].Status != metav1.ConditionTrue {
		t.Fatalf("expected condition status True, got %s", conditions[0].Status)
	}
	if conditions[0].Reason != "LabelPresent" {
		t.Fatalf("expected reason LabelPresent, got %s", conditions[0].Reason)
	}
}

func TestValidateProjectScope_LabelMissing(t *testing.T) {
	cfg := &config.Config{
		ProjectScopeLabel: "zitadel.truvity.com/customer-project",
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "billing",
			Labels: map[string]string{
				"some-other-label": "value",
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

	var conditions []metav1.Condition
	shouldProceed, err := validateProjectScope(context.Background(), k8s, cfg, "billing", &conditions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldProceed {
		t.Fatal("expected shouldProceed=false when label is missing (fail-closed)")
	}
	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conditions))
	}
	if conditions[0].Type != ConditionTypeProjectScope {
		t.Fatalf("expected condition type %q, got %q", ConditionTypeProjectScope, conditions[0].Type)
	}
	if conditions[0].Status != metav1.ConditionFalse {
		t.Fatalf("expected condition status False, got %s", conditions[0].Status)
	}
	if conditions[0].Reason != "MissingLabel" {
		t.Fatalf("expected reason MissingLabel, got %s", conditions[0].Reason)
	}
}

func TestValidateProjectScope_NamespaceNotFound(t *testing.T) {
	cfg := &config.Config{
		ProjectScopeLabel: "zitadel.truvity.com/employee-project",
	}

	// No namespace object registered — lookup will fail.
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()

	var conditions []metav1.Condition
	shouldProceed, err := validateProjectScope(context.Background(), k8s, cfg, "nonexistent", &conditions)
	if err == nil {
		t.Fatal("expected error when namespace does not exist")
	}
	if shouldProceed {
		t.Fatal("expected shouldProceed=false when namespace lookup fails")
	}
}
