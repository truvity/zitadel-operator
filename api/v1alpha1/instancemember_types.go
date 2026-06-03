package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// InstanceMemberSpec defines the desired state of InstanceMember.
type InstanceMemberSpec struct {
	// UserEmail is the email of the Zitadel user to add as an instance member.
	UserEmail string `json:"userEmail"`

	// Roles is the list of roles for this instance member (e.g., IAM_OWNER).
	Roles []string `json:"roles"`
}

// InstanceMemberStatus defines the observed state of InstanceMember.
type InstanceMemberStatus struct {
	// UserId is the resolved Zitadel user ID.
	UserId string `json:"userId,omitempty"`

	// Ready indicates whether the InstanceMember is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Email",type=string,JSONPath=`.spec.userEmail`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// InstanceMember is the Schema for the instancemembers API.
type InstanceMember struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InstanceMemberSpec   `json:"spec,omitempty"`
	Status InstanceMemberStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InstanceMemberList contains a list of InstanceMember.
type InstanceMemberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InstanceMember `json:"items"`
}
