package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OrgMemberSpec defines the desired state of OrgMember.
type OrgMemberSpec struct {
	// OrganizationRef references an Organization CR managed by this operator.
	// Mutually exclusive with OrganizationId.
	// +optional
	OrganizationRef *ResourceRef `json:"organizationRef,omitempty"`

	// OrganizationId references a pre-existing Zitadel organization by raw ID.
	// Mutually exclusive with OrganizationRef.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	// UserRef references a MachineUser or HumanUser CR.
	// +optional
	UserRef *ResourceRef `json:"userRef,omitempty"`

	// UserId is a raw Zitadel user ID.
	// +optional
	UserId string `json:"userId,omitempty"`

	// Roles is the list of roles to assign (e.g., ORG_OWNER, ORG_ADMIN).
	Roles []string `json:"roles"`
}

// OrgMemberStatus defines the observed state of OrgMember.
type OrgMemberStatus struct {
	// OrganizationId is the resolved organization ID.
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the OrgMember is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// OrgMember is the Schema for the orgmembers API.
// It manages an org-scoped membership (Management API).
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
