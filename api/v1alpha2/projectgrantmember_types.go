package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectGrantMemberSpec defines the desired state of ProjectGrantMember.
type ProjectGrantMemberSpec struct {
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

	// GrantId is the Zitadel project grant ID (from ProjectGrant status).
	GrantId string `json:"grantId"`

	// UserRef references a MachineUser CR managed by this operator.
	// Mutually exclusive with UserId.
	// +optional
	UserRef *ResourceRef `json:"userRef,omitempty"`

	// UserId references a pre-existing Zitadel user by raw ID.
	// Mutually exclusive with UserRef.
	// +optional
	UserId string `json:"userId,omitempty"`

	// Roles is the list of roles to assign to the grant member.
	Roles []string `json:"roles"`
}

// ProjectGrantMemberStatus defines the observed state of ProjectGrantMember.
type ProjectGrantMemberStatus struct {
	// Ready indicates whether the ProjectGrantMember is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="GrantID",type=string,JSONPath=`.spec.grantId`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// ProjectGrantMember is the Schema for the projectgrantmembers API.
type ProjectGrantMember struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectGrantMemberSpec   `json:"spec,omitempty"`
	Status ProjectGrantMemberStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectGrantMemberList contains a list of ProjectGrantMember.
type ProjectGrantMemberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProjectGrantMember `json:"items"`
}
