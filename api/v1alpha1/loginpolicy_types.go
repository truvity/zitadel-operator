package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LoginPolicyCRDSpec defines the desired state of LoginPolicy.
type LoginPolicyCRDSpec struct {
	// AllowRegister determines whether self-registration is allowed.
	AllowRegister bool `json:"allowRegister,omitempty"`

	// AllowExternalIdp determines whether external identity providers are allowed.
	AllowExternalIdp bool `json:"allowExternalIdp,omitempty"`

	// HidePasswordReset hides the password reset option on the login page.
	HidePasswordReset bool `json:"hidePasswordReset,omitempty"`

	// DisableLoginWithEmail disables login with email.
	DisableLoginWithEmail bool `json:"disableLoginWithEmail,omitempty"`

	// DisableLoginWithPhone disables login with phone.
	DisableLoginWithPhone bool `json:"disableLoginWithPhone,omitempty"`

	// PasswordlessType is the optional passwordless authentication type.
	// +optional
	PasswordlessType string `json:"passwordlessType,omitempty"`

	// MfaType is the optional multi-factor authentication type.
	// +optional
	MfaType string `json:"mfaType,omitempty"`

	// IdpProviders is a list of IdentityProvider CR names to activate on the login page.
	// The controller resolves these names to Zitadel IdP IDs via the IdentityProvider CR status.
	// +optional
	IdpProviders []string `json:"idpProviders,omitempty"`
}

// LoginPolicyCRDStatus defines the observed state of LoginPolicy.
type LoginPolicyCRDStatus struct {
	// Ready indicates whether the LoginPolicy is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// LoginPolicy is the Schema for the loginpolicies API.
type LoginPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoginPolicyCRDSpec   `json:"spec,omitempty"`
	Status LoginPolicyCRDStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LoginPolicyList contains a list of LoginPolicy.
type LoginPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoginPolicy `json:"items"`
}
