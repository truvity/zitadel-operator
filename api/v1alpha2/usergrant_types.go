package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UserGrantSpec defines the desired state of UserGrant.
type UserGrantSpec struct {
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

	// UserID is the Zitadel user ID to grant roles to.
	// +optional
	UserID string `json:"userId,omitempty"`

	// UserRef references a MachineUser CR managed by this operator.
	// Mutually exclusive with UserID.
	// +optional
	UserRef *ResourceRef `json:"userRef,omitempty"`

	// ProjectRef references a Project CR managed by this operator.
	// Mutually exclusive with ProjectId.
	// +optional
	ProjectRef *ResourceRef `json:"projectRef,omitempty"`

	// ProjectId references a pre-existing Zitadel project by raw ID.
	// Mutually exclusive with ProjectRef.
	// +optional
	ProjectId string `json:"projectId,omitempty"`

	// RoleKeys is the list of role keys to grant.
	RoleKeys []string `json:"roleKeys"`
}

// UserGrantStatus defines the observed state of UserGrant.
type UserGrantStatus struct {
	// GrantId is the Zitadel user grant ID.
	GrantId string `json:"grantId,omitempty"`

	// Ready indicates whether the UserGrant is successfully synced.
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
// +kubebuilder:printcolumn:name="GrantID",type=string,JSONPath=`.status.grantId`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// UserGrant is the Schema for the usergrants API.
type UserGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserGrantSpec   `json:"spec,omitempty"`
	Status UserGrantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UserGrantList contains a list of UserGrant.
type UserGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UserGrant `json:"items"`
}
