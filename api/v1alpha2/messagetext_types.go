package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MessageTextSpec defines the desired state of MessageText.
type MessageTextSpec struct {
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

	MessageTextFields `json:",inline"`
}

// MessageTextStatus defines the observed state of MessageText.
type MessageTextStatus struct {
	// OrganizationId is the resolved organization ID.
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the MessageText is successfully synced.
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
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Language",type=string,JSONPath=`.spec.language`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// MessageText is the Schema for the messagetexts API.
// It manages org-scoped custom message text (ORG_OWNER, Management API). One CR per type+language+org combination.
type MessageText struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MessageTextSpec   `json:"spec,omitempty"`
	Status MessageTextStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MessageTextList contains a list of MessageText.
type MessageTextList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MessageText `json:"items"`
}
