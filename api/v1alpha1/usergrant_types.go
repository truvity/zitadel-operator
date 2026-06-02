package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UserGrantSpec defines the desired state of UserGrant.
type UserGrantSpec struct {
	// UserId is the Zitadel user ID to grant roles to.
	UserId string `json:"userId"`

	// ProjectRef is the name of the Zitadel Project CR this grant belongs to.
	ProjectRef string `json:"projectRef"`

	// RoleKeys is the list of role keys granted to the user.
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
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="User",type=string,JSONPath=`.spec.userId`
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.projectRef`
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
