package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MachineUserSpec defines the desired state of MachineUser.
type MachineUserSpec struct {
	// OrganizationRef references an Organization CR managed by this operator.
	// Mutually exclusive with OrganizationId.
	// +optional
	OrganizationRef *ResourceRef `json:"organizationRef,omitempty"`

	// OrganizationId references a pre-existing Zitadel organization by raw ID.
	// Mutually exclusive with OrganizationRef.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	// UserName is the login name for the machine user.
	UserName string `json:"userName"`

	// Name is the display name of the machine user.
	// If empty, the Kubernetes resource name is used.
	// +optional
	Name string `json:"name,omitempty"`

	// Description is an optional description of the machine user.
	// +optional
	Description string `json:"description,omitempty"`

	// AccessTokenType specifies the access token format for this machine user.
	// +kubebuilder:validation:Enum=bearer;jwt
	// +optional
	AccessTokenType string `json:"accessTokenType,omitempty"`

	// KeySecretRef references the Secret where the generated key JSON will be stored.
	KeySecretRef MachineKeySecretRef `json:"keySecretRef"`
}

// MachineUserStatus defines the observed state of MachineUser.
type MachineUserStatus struct {
	// UserId is the Zitadel user ID.
	UserId string `json:"userId,omitempty"`

	// OrganizationId is the resolved organization ID.
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the MachineUser is successfully synced.
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
// +kubebuilder:printcolumn:name="UserID",type=string,JSONPath=`.status.userId`
// +kubebuilder:printcolumn:name="UserName",type=string,JSONPath=`.spec.userName`
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

// DisplayName returns the machine user display name.
// Falls back to the Kubernetes resource name if spec.name is empty.
func (m *MachineUser) DisplayName() string {
	if m.Spec.Name != "" {
		return m.Spec.Name
	}
	return m.Name
}
