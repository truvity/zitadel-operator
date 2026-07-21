package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectRoleSpec defines the desired state of ProjectRole.
type ProjectRoleSpec struct {
	// Instance optionally pins this resource to one operator's instance
	// identity — the operator config's instanceAlias, defaulting to its
	// domain (v0.18 dual-serving). When set to another identity, the CR is
	// ignored entirely so the owning operator can manage it. When empty
	// while the namespace is served by two operators, both fail closed with
	// an AmbiguousInstance condition.
	// +optional
	Instance string `json:"instance,omitempty"`

	// ProjectRef references a Project CR in the same namespace (or another
	// namespace via ref.namespace). The role is created in that project once
	// it reports a projectId in status.
	// Mutually exclusive with ProjectId.
	// +optional
	ProjectRef *ResourceRef `json:"projectRef,omitempty"`

	// ProjectId references a pre-existing Zitadel project by raw ID.
	// Mutually exclusive with ProjectRef.
	// +optional
	ProjectId string `json:"projectId,omitempty"`

	// Key is the role key used in authorization checks and token claims.
	// If empty, the Kubernetes resource name is used. The key cannot be
	// changed in Zitadel; changing it here removes the old role (including
	// its grants) and creates a new one.
	// +optional
	Key string `json:"key,omitempty"`

	// DisplayName is the human-readable name of the role.
	// If empty, the role key is used.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Group optionally groups roles for display purposes (not a collection
	// of users; Zitadel does not evaluate it).
	// +optional
	Group string `json:"group,omitempty"`
}

// ProjectRoleStatus defines the observed state of ProjectRole.
type ProjectRoleStatus struct {
	// ProjectId is the resolved Zitadel project ID the role belongs to.
	ProjectId string `json:"projectId,omitempty"`

	// Key is the role key currently reconciled into the project. Kept so a
	// key change can remove the previously managed role.
	Key string `json:"key,omitempty"`

	// ObservedGeneration is the spec generation last reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Ready indicates whether the ProjectRole is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Key",type=string,JSONPath=`.status.key`
// +kubebuilder:printcolumn:name="ProjectID",type=string,JSONPath=`.status.projectId`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// ProjectRole is the Schema for the projectroles API.
type ProjectRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectRoleSpec   `json:"spec,omitempty"`
	Status ProjectRoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectRoleList contains a list of ProjectRole.
type ProjectRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProjectRole `json:"items"`
}

// RoleKey returns the effective role key.
// Falls back to the Kubernetes resource name if spec.key is empty.
func (r *ProjectRole) RoleKey() string {
	if r.Spec.Key != "" {
		return r.Spec.Key
	}
	return r.Name
}

// RoleDisplayName returns the effective display name.
// Falls back to the role key if spec.displayName is empty.
func (r *ProjectRole) RoleDisplayName() string {
	if r.Spec.DisplayName != "" {
		return r.Spec.DisplayName
	}
	return r.RoleKey()
}
