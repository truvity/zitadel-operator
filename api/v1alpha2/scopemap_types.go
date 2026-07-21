package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScopeMapRule maps a set of tenant namespaces to a Zitadel scope.
// Exactly one of NamespaceSelector or Namespaces must be set.
type ScopeMapRule struct {
	// Name identifies the rule for status reporting and Events.
	Name string `json:"name"`

	// NamespaceSelector selects namespaces by label.
	// Mutually exclusive with Namespaces.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// Namespaces is a literal list of namespace names.
	// Mutually exclusive with NamespaceSelector.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// Project is the Zitadel project name this rule scopes to. The project
	// is created on first use when it does not exist. When both Project and
	// ProjectId are empty, the rule grants org-scope.
	// Names are for humans, IDs for machines: when ProjectId is set it is
	// authoritative and Project is informational.
	// +optional
	Project string `json:"project,omitempty"`

	// ProjectId pins the Zitadel project by raw ID (authoritative when set;
	// the project must already exist). May be used with or without a
	// Project name.
	// +optional
	ProjectId string `json:"projectId,omitempty"`
}

// ScopeMapSpec defines the desired state of ScopeMap.
type ScopeMapSpec struct {
	// Instance is the operator instance identity this map applies to — the
	// operator config's instanceAlias, defaulting to its domain. Must match
	// the serving operator's identity; otherwise the map is fail-closed with
	// an InstanceMismatch condition.
	Instance string `json:"instance"`

	// Organization is the Zitadel organization name. Required when
	// OrganizationId is empty (the map controller resolves the name to an
	// ID); optional and informational when OrganizationId is set.
	// +optional
	Organization string `json:"organization,omitempty"`

	// OrganizationId pins the Zitadel organization by raw ID.
	// When set, the ID is authoritative; a differing Organization name is
	// reported as drift (OrganizationNameDrift condition + Event, not an
	// error). At least one of Organization / OrganizationId must be set.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	// Rules is the ordered rule list. First match top-down wins
	// (evaluated across all maps in the operator namespace).
	// +kubebuilder:validation:MinItems=1
	Rules []ScopeMapRule `json:"rules"`
}

// ScopeMapStatus defines the observed state of ScopeMap.
type ScopeMapStatus struct {
	// Ready indicates the map passed validation (instance match, org resolved).
	Ready bool `json:"ready,omitempty"`

	// ResolvedOrganizationId is the org ID resolved from spec
	// (spec.organizationId when set, otherwise looked up by name).
	ResolvedOrganizationId string `json:"resolvedOrganizationId,omitempty"`

	// ObservedGeneration is the last generation reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the map's state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instance`
// +kubebuilder:printcolumn:name="Organization",type=string,JSONPath=`.spec.organization`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// ScopeMap maps tenant namespaces to Zitadel org/project scopes.
// Namespaced; only maps in the operator's own namespace are evaluated.
type ScopeMap struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScopeMapSpec   `json:"spec,omitempty"`
	Status ScopeMapStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScopeMapList contains a list of ScopeMap.
type ScopeMapList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScopeMap `json:"items"`
}
