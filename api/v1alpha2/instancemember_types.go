package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InstanceMemberSpec defines the desired state of InstanceMember.
type InstanceMemberSpec struct {
	// UserRef references a MachineUser or HumanUser CR.
	// +optional
	UserRef *ResourceRef `json:"userRef,omitempty"`

	// UserId is a raw Zitadel user ID.
	// +optional
	UserId string `json:"userId,omitempty"`

	// Roles is the list of instance-level roles (e.g., IAM_OWNER, IAM_ORG_MANAGER).
	Roles []string `json:"roles"`
}

// InstanceMemberStatus defines the observed state of InstanceMember.
type InstanceMemberStatus struct {
	// Ready indicates whether the InstanceMember is successfully synced.
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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// InstanceMember is the Schema for the instancemembers API.
// It manages an instance-level membership (Admin API).
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
