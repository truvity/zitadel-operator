package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectMemberSpec defines the desired state of ProjectMember.
type ProjectMemberSpec struct {
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

	// UserRef references a MachineUser CR managed by this operator.
	// Mutually exclusive with UserId.
	// +optional
	UserRef *ResourceRef `json:"userRef,omitempty"`

	// UserId is the Zitadel user ID to add as a project member.
	// Mutually exclusive with UserRef.
	// +optional
	UserId string `json:"userId,omitempty"`

	// Roles is the list of roles to assign (e.g., PROJECT_OWNER, PROJECT_OWNER_VIEWER).
	Roles []string `json:"roles"`
}

// ProjectMemberStatus defines the observed state of ProjectMember.
type ProjectMemberStatus struct {
	// Ready indicates whether the ProjectMember is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// ProjectMember is the Schema for the projectmembers API.
type ProjectMember struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectMemberSpec   `json:"spec,omitempty"`
	Status ProjectMemberStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectMemberList contains a list of ProjectMember.
type ProjectMemberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProjectMember `json:"items"`
}
