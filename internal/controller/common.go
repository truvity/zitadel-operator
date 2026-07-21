// Package controller implements Kubernetes controllers for Zitadel resources.
package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

const (
	// finalizerName is the finalizer used by all Zitadel controllers.
	finalizerName = "zitadel.truvity.io/finalizer"

	// requeueInterval is the default requeue interval for periodic reconciliation.
	requeueInterval = 5 * time.Minute

	// requeueOnError is the requeue interval for transient errors (ref not ready, etc).
	requeueOnError = 10 * time.Second

	// ConditionTypeReady indicates the resource is fully reconciled.
	ConditionTypeReady = "Ready"

	// ConditionTypeSynced indicates the resource has been synced with Zitadel.
	ConditionTypeSynced = "Synced"

	// AnnotationResetOnDelete controls whether instance-default singletons
	// reset to baseline values when the CR is deleted.
	// Default: "false" (leave instance state untouched on delete).
	AnnotationResetOnDelete = "zitadel.truvity.io/reset-on-delete"
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

// shouldResetOnDelete returns true if the CR has the reset-on-delete annotation set to "true".
// Instance-default singleton policies do NOT mutate instance state on delete by default —
// they simply stop managing the resource. Opt-in via annotation for explicit reset.
func shouldResetOnDelete(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	return annotations[AnnotationResetOnDelete] == "true"
}

// generationChangedPredicate returns a predicate that filters out status-only
// updates. Only spec changes (generation bump) and deletion trigger reconciliation.
// This prevents hot-loops where status writes trigger re-reconciliation.
func generationChangedPredicate() predicate.Predicate {
	return predicate.GenerationChangedPredicate{}
}

// resolveOrganizationId resolves the organization ID from either an
// OrganizationRef or an explicit OrganizationId (v0.18 removed the config
// default-org fallback).
func resolveOrganizationId(ctx context.Context, k8s client.Client, ref *zitadelv1alpha2.ResourceRef, explicitID, sourceNamespace string) (string, error) {
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

	return "", fmt.Errorf("no organization specified: set organizationRef or organizationId (v0.18 removed defaultOrganizationId; namespaces routed by a ZitadelScopeMap inherit the scope's organization)")
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

// setCondition sets or updates a condition in the given conditions slice.
// If a condition of the given type exists, it is updated; otherwise a new one is appended.
// LastTransitionTime is only updated when the status or reason actually changes.
func setCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	for i, c := range *conditions {
		if c.Type == conditionType {
			if c.Status != status || c.Reason != reason {
				(*conditions)[i].Status = status
				(*conditions)[i].Reason = reason
				(*conditions)[i].Message = message
				(*conditions)[i].LastTransitionTime = now
			} else {
				(*conditions)[i].Message = message
			}
			return
		}
	}
	*conditions = append(*conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}

// singletonCandidate holds the minimal info needed for conflict detection.
type singletonCandidate struct {
	UID               types.UID
	Name              string
	Namespace         string
	CreationTimestamp metav1.Time
	IsDeleting        bool
}

// checkSingletonConflict determines if this CR should yield to an earlier-created CR of the same kind.
// Returns true if the CR is a duplicate (conditions and ready status updated), false if it should proceed.
// CreationTimestamp has 1s granularity, so equal timestamps are tie-broken by
// namespace/name ordering to keep the winner deterministic.
func checkSingletonConflict(cr client.Object, candidates []singletonCandidate, conditions *[]metav1.Condition, ready *bool, kindName string) bool {
	crTime := cr.GetCreationTimestamp()
	for _, other := range candidates {
		if other.UID == cr.GetUID() {
			continue
		}
		earlier := other.CreationTimestamp.Before(&crTime) ||
			(other.CreationTimestamp.Equal(&crTime) &&
				other.Namespace+"/"+other.Name < cr.GetNamespace()+"/"+cr.GetName())
		if earlier && !other.IsDeleting {
			setCondition(conditions, ConditionTypeReady, metav1.ConditionFalse, "DuplicateSingleton",
				fmt.Sprintf("another %s %s/%s (created earlier) is already managing this instance singleton", kindName, other.Namespace, other.Name))
			*ready = false
			return true
		}
	}
	return false
}

// resolveAppId resolves an application ID from either an AppRef or explicit AppId.
// It looks up OIDCApp first, then APIApp if not found.
func resolveAppId(ctx context.Context, k8s client.Client, ref *zitadelv1alpha2.ResourceRef, explicitID, sourceNamespace string) (string, error) {
	if ref != nil && explicitID != "" {
		return "", fmt.Errorf("appRef and appId are mutually exclusive")
	}

	if ref == nil && explicitID == "" {
		return "", fmt.Errorf("one of appRef or appId is required")
	}

	if explicitID != "" {
		return explicitID, nil
	}

	ns := ref.Namespace
	if ns == "" {
		ns = sourceNamespace
	}

	// Try OIDCApp first.
	var oidcApp zitadelv1alpha2.OIDCApp
	if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, &oidcApp); err == nil {
		if oidcApp.Status.ApplicationId == "" {
			return "", fmt.Errorf("appRef %s/%s not yet ready (no applicationId in status)", ns, ref.Name)
		}
		return oidcApp.Status.ApplicationId, nil
	}

	// Try APIApp.
	var apiApp zitadelv1alpha2.APIApp
	if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, &apiApp); err == nil {
		if apiApp.Status.ApplicationId == "" {
			return "", fmt.Errorf("appRef %s/%s not yet ready (no applicationId in status)", ns, ref.Name)
		}
		return apiApp.Status.ApplicationId, nil
	}

	return "", fmt.Errorf("appRef %s/%s not found (tried OIDCApp and APIApp)", ns, ref.Name)
}

// resolveUserId resolves a user ID from either a UserRef or explicit UserId.
func resolveUserId(ctx context.Context, k8s client.Client, ref *zitadelv1alpha2.ResourceRef, explicitID, sourceNamespace string) (string, error) {
	if ref != nil && explicitID != "" {
		return "", fmt.Errorf("userRef and userId are mutually exclusive")
	}

	if ref == nil && explicitID == "" {
		return "", fmt.Errorf("one of userRef or userId is required")
	}

	if explicitID != "" {
		return explicitID, nil
	}

	ns := ref.Namespace
	if ns == "" {
		ns = sourceNamespace
	}
	var mu zitadelv1alpha2.MachineUser
	if err := k8s.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, &mu); err != nil {
		return "", fmt.Errorf("resolving userRef %s/%s: %w", ns, ref.Name, err)
	}
	if mu.Status.UserId == "" {
		return "", fmt.Errorf("userRef %s/%s not yet ready (no userId in status)", ns, ref.Name)
	}
	return mu.Status.UserId, nil
}
