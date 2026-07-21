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

	// Project is the Zitadel project name this rule scopes to.
	// When empty, the rule grants org-scope.
	// +optional
	Project string `json:"project,omitempty"`

	// ProjectId pins the Zitadel project by raw ID.
	// Only valid together with a literal Project name.
	// +optional
	ProjectId string `json:"projectId,omitempty"`
}

// ZitadelScopeMapSpec defines the desired state of ZitadelScopeMap.
type ZitadelScopeMapSpec struct {
	// Instance is the Zitadel instance domain this map applies to.
	// Must match the operator's binding domain; otherwise the map is
	// fail-closed with an InstanceMismatch condition.
	Instance string `json:"instance"`

	// Organization is the Zitadel organization name.
	Organization string `json:"organization"`

	// OrganizationId pins the Zitadel organization by raw ID.
	// When set, the ID is authoritative; a differing Organization name
	// is reported as drift via an Event (not an error).
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	// Rules is the ordered rule list. First match top-down wins
	// (evaluated across all maps in the operator namespace).
	// +kubebuilder:validation:MinItems=1
	Rules []ScopeMapRule `json:"rules"`
}

// ZitadelScopeMapStatus defines the observed state of ZitadelScopeMap.
type ZitadelScopeMapStatus struct {
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

// ZitadelScopeMap maps tenant namespaces to Zitadel org/project scopes.
// Namespaced; only maps in the operator's own namespace are evaluated.
type ZitadelScopeMap struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZitadelScopeMapSpec   `json:"spec,omitempty"`
	Status ZitadelScopeMapStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZitadelScopeMapList contains a list of ZitadelScopeMap.
type ZitadelScopeMapList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZitadelScopeMap `json:"items"`
}
