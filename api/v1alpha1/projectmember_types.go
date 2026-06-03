package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectMemberSpec defines the desired state of ProjectMember.
type ProjectMemberSpec struct {
	// ProjectRef is the name of the Zitadel Project CR this member belongs to.
	ProjectRef string `json:"projectRef"`

	// UserId is the Zitadel user ID to add as a project member.
	// +optional
	UserId string `json:"userId,omitempty"`

	// UserEmail is an alternative to UserId. When set, the controller resolves the email to a Zitadel user ID.
	// The user must have logged in at least once.
	// +optional
	UserEmail string `json:"userEmail,omitempty"`

	// Roles is the list of roles for this project member (e.g. PROJECT_OWNER).
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
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.projectRef`
// +kubebuilder:printcolumn:name="User",type=string,JSONPath=`.spec.userId`
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
