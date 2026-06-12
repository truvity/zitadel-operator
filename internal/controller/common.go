// Package controller implements Kubernetes controllers for Zitadel resources.
package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
)

const (
	// finalizerName is the finalizer used by all Zitadel controllers.
	finalizerName = "zitadel.truvity.io/finalizer"

	// requeueInterval is the default requeue interval for periodic reconciliation.
	requeueInterval = 5 * time.Minute

	// requeueOnError is the requeue interval for transient errors (ref not ready, etc).
	requeueOnError = 10 * time.Second
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

// generationChangedPredicate returns a predicate that filters out status-only
// updates. Only spec changes (generation bump) and deletion trigger reconciliation.
// This prevents hot-loops where status writes trigger re-reconciliation.
func generationChangedPredicate() predicate.Predicate {
	return predicate.GenerationChangedPredicate{}
}

// resolveOrganizationId resolves the organization ID from either an OrganizationRef,
// an explicit OrganizationId, or the operator's default organization ID.
func resolveOrganizationId(ctx context.Context, k8s client.Client, cfg *config.Config, ref *zitadelv1alpha2.ResourceRef, explicitID, sourceNamespace string) (string, error) {
	if ref != nil && explicitID != "" {
		return "", fmt.Errorf("organizationRef and organizationId are mutually exclusive")
	}

	if explicitID != "" {
		return explicitID, nil
	}

	if ref != nil {
		ns := ref.Namespace
		if ns == "" {
			ns = sourceNamespace
		}
		var org zitadelv1alpha2.Organization
		if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, &org); err != nil {
			return "", fmt.Errorf("resolving organizationRef %s/%s: %w", ns, ref.Name, err)
		}
		if org.Status.OrganizationId == "" {
			return "", fmt.Errorf("organizationRef %s/%s not yet ready (no organizationId in status)", ns, ref.Name)
		}
		return org.Status.OrganizationId, nil
	}

	if cfg.DefaultOrganizationId != "" {
		return cfg.DefaultOrganizationId, nil
	}

	return "", fmt.Errorf("no organization specified: set organizationRef, organizationId, or configure defaultOrganizationId")
}

// resolveProjectId resolves the project ID from either a ProjectRef or explicit ProjectId.
func resolveProjectId(ctx context.Context, k8s client.Client, ref *zitadelv1alpha2.ResourceRef, explicitID, sourceNamespace string) (projectID, orgID string, err error) {
	if ref != nil && explicitID != "" {
		return "", "", fmt.Errorf("projectRef and projectId are mutually exclusive")
	}

	if ref == nil && explicitID == "" {
		return "", "", fmt.Errorf("one of projectRef or projectId is required")
	}

	if explicitID != "" {
		return explicitID, "", nil
	}

	ns := ref.Namespace
	if ns == "" {
		ns = sourceNamespace
	}
	var proj zitadelv1alpha2.Project
	if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, &proj); err != nil {
		return "", "", fmt.Errorf("resolving projectRef %s/%s: %w", ns, ref.Name, err)
	}
	if proj.Status.ProjectId == "" {
		return "", "", fmt.Errorf("projectRef %s/%s not yet ready (no projectId in status)", ns, ref.Name)
	}
	return proj.Status.ProjectId, proj.Status.OrganizationId, nil
}

// isRefNotReady returns true if the error indicates a referenced resource
// is not yet ready (transient — will resolve once the dependency reconciles).
func isRefNotReady(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not yet ready")
}
