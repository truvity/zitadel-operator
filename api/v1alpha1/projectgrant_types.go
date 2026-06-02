package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectGrantSpec defines the desired state of ProjectGrant.
type ProjectGrantSpec struct {
	// ProjectRef is the name of the Zitadel Project CR this grant belongs to.
	ProjectRef string `json:"projectRef"`

	// GrantedOrgId is the organization ID that receives the project grant.
	GrantedOrgId string `json:"grantedOrgId"`

	// RoleKeys is the list of role keys granted to the organization.
	// +optional
	RoleKeys []string `json:"roleKeys,omitempty"`
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
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.projectRef`
// +kubebuilder:printcolumn:name="GrantedOrg",type=string,JSONPath=`.spec.grantedOrgId`
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
