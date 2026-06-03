package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// OrgMemberSpec defines the desired state of OrgMember.
type OrgMemberSpec struct {
	// UserEmail is the email of the Zitadel user to add as an org member.
	UserEmail string `json:"userEmail"`

	// Roles is the list of roles for this org member (e.g., ORG_OWNER).
	Roles []string `json:"roles"`
}

// OrgMemberStatus defines the observed state of OrgMember.
type OrgMemberStatus struct {
	// UserId is the resolved Zitadel user ID.
	UserId string `json:"userId,omitempty"`

	// Ready indicates whether the OrgMember is successfully synced.
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

// OrgMember is the Schema for the orgmembers API.
type OrgMember struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrgMemberSpec   `json:"spec,omitempty"`
	Status OrgMemberStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OrgMemberList contains a list of OrgMember.
type OrgMemberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrgMember `json:"items"`
}
