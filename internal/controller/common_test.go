package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
)

func TestAddFinalizer(t *testing.T) {
	cr := &zitadelv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}

	// First call should add and return true.
	if !addFinalizer(cr) {
		t.Fatal("expected true (finalizer added)")
	}

	if !controllerutil.ContainsFinalizer(cr, finalizerName) {
		t.Fatal("finalizer should be present")
	}

	// Second call should return false (already present).
	if addFinalizer(cr) {
		t.Fatal("expected false (finalizer already present)")
	}
}

func TestRemoveFinalizer(t *testing.T) {
	cr := &zitadelv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Finalizers: []string{finalizerName},
		},
	}

	// First call should remove and return true.
	if !removeFinalizer(cr) {
		t.Fatal("expected true (finalizer removed)")
	}

	if controllerutil.ContainsFinalizer(cr, finalizerName) {
		t.Fatal("finalizer should be gone")
	}

	// Second call should return false (already gone).
	if removeFinalizer(cr) {
		t.Fatal("expected false (finalizer already gone)")
	}
}

func TestFinalizerName(t *testing.T) {
	if finalizerName != "zitadel.truvity.io/finalizer" {
		t.Fatalf("unexpected finalizer name: %s", finalizerName)
	}
}
