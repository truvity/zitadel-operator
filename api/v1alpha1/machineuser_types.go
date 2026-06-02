package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MachineUserSpec defines the desired state of MachineUser.
type MachineUserSpec struct {
	// Username is the login name for the machine user.
	Username string `json:"username"`

	// Name is the display name of the machine user.
	Name string `json:"name"`

	// Description is an optional description of the machine user.
	// +optional
	Description string `json:"description,omitempty"`

	// ProjectRef is an optional reference to a Project to grant PROJECT_OWNER role.
	// +optional
	ProjectRef string `json:"projectRef,omitempty"`

	// KeySecretRef references the Secret where the generated key JSON will be stored.
	KeySecretRef SecretRefSpec `json:"keySecretRef"`

	// KeyExpirationDays is the number of days until the machine key expires.
	// Defaults to 3650 (10 years) if not set.
	// +optional
	KeyExpirationDays int `json:"keyExpirationDays,omitempty"`
}

// MachineUserStatus defines the observed state of MachineUser.
type MachineUserStatus struct {
	// UserId is the Zitadel user ID.
	UserId string `json:"userId,omitempty"`

	// Ready indicates whether the MachineUser is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// MachineUser is the Schema for the machineusers API.
type MachineUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineUserSpec   `json:"spec,omitempty"`
	Status MachineUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MachineUserList contains a list of MachineUser.
type MachineUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineUser `json:"items"`
}
