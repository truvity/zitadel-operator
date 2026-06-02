// Package controller implements Kubernetes controllers for Zitadel resources.
package controller

import (
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// finalizerName is the finalizer used by all Zitadel controllers.
	finalizerName = "zitadel.truvity.io/finalizer"

	// requeueInterval is the default requeue interval for periodic reconciliation.
	requeueInterval = 5 * time.Minute
)

// addFinalizer adds the finalizer to the object if not already present.
func addFinalizer(obj client.Object) bool {
	if !controllerutil.ContainsFinalizer(obj, finalizerName) {
		controllerutil.AddFinalizer(obj, finalizerName)
		return true
	}
	return false
}

// removeFinalizer removes the finalizer from the object.
func removeFinalizer(obj client.Object) bool {
	if controllerutil.ContainsFinalizer(obj, finalizerName) {
		controllerutil.RemoveFinalizer(obj, finalizerName)
		return true
	}
	return false
}
