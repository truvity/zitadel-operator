package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrivacyPolicySpec defines the desired state of PrivacyPolicy.
type PrivacyPolicySpec struct {
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

	PrivacyPolicyFields `json:",inline"`
}

// PrivacyPolicyStatus defines the observed state of PrivacyPolicy.
type PrivacyPolicyStatus struct {
	// OrganizationId is the resolved organization ID.
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the PrivacyPolicy is successfully synced.
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

// PrivacyPolicy is the Schema for the privacypolicies API.
// It manages an org-scoped privacy policy (Management API).
type PrivacyPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrivacyPolicySpec   `json:"spec,omitempty"`
	Status PrivacyPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PrivacyPolicyList contains a list of PrivacyPolicy.
type PrivacyPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrivacyPolicy `json:"items"`
}
