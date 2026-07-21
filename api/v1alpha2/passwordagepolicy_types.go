package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PasswordAgePolicySpec defines the desired state of PasswordAgePolicy.
type PasswordAgePolicySpec struct {
	// Instance optionally pins this resource to one operator's instance
	// identity — the operator config's instanceAlias, defaulting to its
	// domain (v0.18 dual-serving). When set to another identity, the CR is
	// ignored entirely so the owning operator can manage it. When empty
	// while the namespace is served by two operators, both fail closed with
	// an AmbiguousInstance condition.
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

	PasswordAgePolicyFields `json:",inline"`
}

// PasswordAgePolicyStatus defines the observed state of PasswordAgePolicy.
type PasswordAgePolicyStatus struct {
	// OrganizationId is the resolved organization ID.
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the PasswordAgePolicy is successfully synced.
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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// PasswordAgePolicy is the Schema for the passwordagepolicies API.
// It manages an org-scoped password age policy (Management API).
type PasswordAgePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PasswordAgePolicySpec   `json:"spec,omitempty"`
	Status PasswordAgePolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PasswordAgePolicyList contains a list of PasswordAgePolicy.
type PasswordAgePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PasswordAgePolicy `json:"items"`
}
