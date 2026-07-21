package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PasswordComplexityPolicySpec defines the desired state of PasswordComplexityPolicy (org-scoped).
type PasswordComplexityPolicySpec struct {
	// Instance optionally pins this resource to a specific Zitadel instance
	// domain (v0.18 dual-serving). When set to a domain other than this
	// operator's binding, the CR is ignored entirely so the owning operator
	// can manage it. When empty while the namespace is served by two
	// operators, both fail closed with an AmbiguousInstance condition.
	// +optional
	Instance string `json:"instance,omitempty"`

	// OrganizationRef references an Organization CR managed by this operator.
	// Mutually exclusive with OrganizationId.
	// +optional
	OrganizationRef *ResourceRef `json:"organizationRef,omitempty"`

	// OrganizationId references a pre-existing Zitadel organization by raw ID.
	// Mutually exclusive with OrganizationRef.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	PasswordComplexityPolicyFields `json:",inline"`
}

// PasswordComplexityPolicyStatus defines the observed state of PasswordComplexityPolicy.
type PasswordComplexityPolicyStatus struct {
	// Ready indicates whether the PasswordComplexityPolicy is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// OrganizationId is the resolved organization ID.
	OrganizationId string `json:"organizationId,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// PasswordComplexityPolicy is the Schema for the passwordcomplexitypolicies API.
// It manages an org-scoped password complexity policy (ORG_OWNER, Management API).
type PasswordComplexityPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PasswordComplexityPolicySpec   `json:"spec,omitempty"`
	Status PasswordComplexityPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PasswordComplexityPolicyList contains a list of PasswordComplexityPolicy.
type PasswordComplexityPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PasswordComplexityPolicy `json:"items"`
}
