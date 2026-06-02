package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LoginPolicySpec defines login policy settings for an identity provider.
type LoginPolicySpec struct {
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

	// DefaultRedirectUri is the default redirect URI after login.
	DefaultRedirectUri string `json:"defaultRedirectUri,omitempty"`
}

// IdentityProviderSpec defines the desired state of IdentityProvider.
type IdentityProviderSpec struct {
	// Type is the identity provider type (google, github, or saml).
	// +kubebuilder:validation:Enum=google;github;saml
	Type string `json:"type"`

	// ClientIdFromSecret references the Secret containing the client ID.
	ClientIdFromSecret SecretKeyRef `json:"clientIdFromSecret"`

	// ClientSecretFromSecret references the Secret containing the client secret.
	ClientSecretFromSecret SecretKeyRef `json:"clientSecretFromSecret"`

	// Scopes is the list of OAuth scopes to request.
	Scopes []string `json:"scopes,omitempty"`

	// AutoCreation enables automatic user creation on first login.
	AutoCreation bool `json:"autoCreation,omitempty"`

	// AutoUpdate enables automatic user profile update on login.
	AutoUpdate bool `json:"autoUpdate,omitempty"`

	// LoginPolicy configures login behavior for this identity provider.
	// +optional
	LoginPolicy *LoginPolicySpec `json:"loginPolicy,omitempty"`
}

// IdentityProviderStatus defines the observed state of IdentityProvider.
type IdentityProviderStatus struct {
	// IdpId is the Zitadel identity provider ID.
	IdpId string `json:"idpId,omitempty"`

	// Ready indicates whether the IdentityProvider is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// IdentityProvider is the Schema for the identityproviders API.
type IdentityProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IdentityProviderSpec   `json:"spec,omitempty"`
	Status IdentityProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IdentityProviderList contains a list of IdentityProvider.
type IdentityProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IdentityProvider `json:"items"`
}
