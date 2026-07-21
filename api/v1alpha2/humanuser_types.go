package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HumanUserSpec defines the desired state of HumanUser.
type HumanUserSpec struct {
	// OrganizationRef references an Organization CR managed by this operator.
	// Mutually exclusive with OrganizationId.
	// +optional
	OrganizationRef *ResourceRef `json:"organizationRef,omitempty"`

	// OrganizationId references a pre-existing Zitadel organization by raw ID.
	// Mutually exclusive with OrganizationRef.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	// UserName is the login name of the user.
	UserName string `json:"userName"`

	// FirstName is the user's first name.
	FirstName string `json:"firstName"`

	// LastName is the user's last name.
	LastName string `json:"lastName"`

	// Email is the user's email address.
	Email string `json:"email"`

	// IsEmailVerified marks the email as pre-verified.
	// +optional
	IsEmailVerified bool `json:"isEmailVerified,omitempty"`

	// DisplayName is the user's display name.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// NickName is the user's nickname.
	// +optional
	NickName string `json:"nickName,omitempty"`

	// PreferredLanguage is the user's preferred language (BCP 47).
	// +optional
	PreferredLanguage string `json:"preferredLanguage,omitempty"`

	// Phone is the user's phone number.
	// +optional
	Phone string `json:"phone,omitempty"`

	// IsPhoneVerified marks the phone as pre-verified.
	// +optional
	IsPhoneVerified bool `json:"isPhoneVerified,omitempty"`

	// InitialPasswordSecretRef references a Secret containing the initial password.
	// +optional
	InitialPasswordSecretRef *SecretKeyRef `json:"initialPasswordSecretRef,omitempty"`
}

// HumanUserStatus defines the observed state of HumanUser.
type HumanUserStatus struct {
	// UserId is the Zitadel user ID.
	UserId string `json:"userId,omitempty"`

	// OrganizationId is the resolved organization ID.
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the HumanUser is successfully synced.
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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// HumanUser is the Schema for the humanusers API.
// It manages an org-scoped human user (Management API).
type HumanUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HumanUserSpec   `json:"spec,omitempty"`
	Status HumanUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HumanUserList contains a list of HumanUser.
type HumanUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HumanUser `json:"items"`
}
