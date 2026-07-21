package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectGrantSpec defines the desired state of ProjectGrant.
type ProjectGrantSpec struct {
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

	// ProjectRef references a Project CR managed by this operator.
	// Mutually exclusive with ProjectId.
	// +optional
	ProjectRef *ResourceRef `json:"projectRef,omitempty"`

	// ProjectId references a pre-existing Zitadel project by raw ID.
	// Mutually exclusive with ProjectRef.
	// +optional
	ProjectId string `json:"projectId,omitempty"`

	// GrantedOrgRef references an Organization CR that receives the grant.
	// Mutually exclusive with GrantedOrgId.
	// +optional
	GrantedOrgRef *ResourceRef `json:"grantedOrgRef,omitempty"`

	// GrantedOrgId is the raw Zitadel org ID that receives the grant.
	// Mutually exclusive with GrantedOrgRef.
	// +optional
	GrantedOrgId string `json:"grantedOrgId,omitempty"`

	// RoleKeys is the list of role keys to grant to the target org.
	RoleKeys []string `json:"roleKeys"`
}

// ProjectGrantStatus defines the observed state of ProjectGrant.
type ProjectGrantStatus struct {
	// GrantId is the Zitadel project grant ID.
	GrantId string `json:"grantId,omitempty"`

	// Ready indicates whether the ProjectGrant is successfully synced.
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

// ProjectGrant is the Schema for the projectgrants API.
type ProjectGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectGrantSpec   `json:"spec,omitempty"`
	Status ProjectGrantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectGrantList contains a list of ProjectGrant.
type ProjectGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProjectGrant `json:"items"`
}
