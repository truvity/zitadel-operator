package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LockoutPolicySpec defines the desired state of LockoutPolicy (org-scoped).
type LockoutPolicySpec struct {
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

	LockoutPolicyFields `json:",inline"`
}

// LockoutPolicyStatus defines the observed state of LockoutPolicy.
type LockoutPolicyStatus struct {
	// Ready indicates whether the LockoutPolicy is successfully synced.
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

// LockoutPolicy is the Schema for the lockoutpolicies API.
// It manages an org-scoped lockout policy (ORG_OWNER, Management API).
type LockoutPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LockoutPolicySpec   `json:"spec,omitempty"`
	Status LockoutPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LockoutPolicyList contains a list of LockoutPolicy.
type LockoutPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LockoutPolicy `json:"items"`
}
