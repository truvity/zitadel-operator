package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OrgMetadataSpec defines the desired state of OrgMetadata.
type OrgMetadataSpec struct {
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

	// Key is the metadata key.
	Key string `json:"key"`

	// Value is the metadata value (will be stored as bytes in Zitadel).
	Value string `json:"value"`
}

// OrgMetadataStatus defines the observed state of OrgMetadata.
type OrgMetadataStatus struct {
	// Ready indicates whether the OrgMetadata is successfully synced.
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
// +kubebuilder:printcolumn:name="Key",type=string,JSONPath=`.spec.key`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// OrgMetadata is the Schema for the orgmetadata API.
type OrgMetadata struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrgMetadataSpec   `json:"spec,omitempty"`
	Status OrgMetadataStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OrgMetadataList contains a list of OrgMetadata.
type OrgMetadataList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrgMetadata `json:"items"`
}
