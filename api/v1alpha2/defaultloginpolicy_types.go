package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IdpReference references an identity provider either by CR reference or raw ID.
type IdpReference struct {
	// IdpRef references a GoogleIdP CR managed by this operator.
	// Mutually exclusive with IdpId.
	// +optional
	IdpRef *ResourceRef `json:"idpRef,omitempty"`

	// IdpId references a pre-existing Zitadel identity provider by raw ID.
	// Mutually exclusive with IdpRef.
	// +optional
	IdpId string `json:"idpId,omitempty"`
}

// DefaultLoginPolicySpec defines the desired state of DefaultLoginPolicy.
type DefaultLoginPolicySpec struct {
	// UserLogin determines whether login with username/password is allowed.
	// +optional
	UserLogin *bool `json:"userLogin,omitempty"`

	// AllowExternalIdp allows login with external identity providers.
	// +optional
	AllowExternalIdp *bool `json:"allowExternalIdp,omitempty"`

	// AllowRegister allows user self-registration.
	// +optional
	AllowRegister *bool `json:"allowRegister,omitempty"`

	// ForceMfa requires multi-factor authentication for all users.
	// +optional
	ForceMfa *bool `json:"forceMfa,omitempty"`

	// ForceMfaLocalOnly requires MFA only for local (non-IdP) users.
	// +optional
	ForceMfaLocalOnly *bool `json:"forceMfaLocalOnly,omitempty"`

	// HidePasswordReset hides the "forgot password" link on the login page.
	// +optional
	HidePasswordReset *bool `json:"hidePasswordReset,omitempty"`

	// PasswordlessType configures passwordless authentication.
	// +kubebuilder:validation:Enum=not_allowed;allowed
	// +optional
	PasswordlessType string `json:"passwordlessType,omitempty"`

	// AllowDomainDiscovery enables domain-based organization discovery.
	// +optional
	AllowDomainDiscovery *bool `json:"allowDomainDiscovery,omitempty"`

	// IgnoreUnknownUsernames prevents enumeration attacks by not revealing if a username exists.
	// +optional
	IgnoreUnknownUsernames *bool `json:"ignoreUnknownUsernames,omitempty"`

	// DefaultRedirectUri is the default redirect URI after login.
	// +optional
	DefaultRedirectUri string `json:"defaultRedirectUri,omitempty"`

	// PasswordCheckLifetime is the duration before password re-verification is required (e.g., "240h").
	// +optional
	PasswordCheckLifetime string `json:"passwordCheckLifetime,omitempty"`

	// ExternalLoginCheckLifetime is the duration before external login re-verification (e.g., "240h").
	// +optional
	ExternalLoginCheckLifetime string `json:"externalLoginCheckLifetime,omitempty"`

	// MfaInitSkipLifetime is the duration a user can skip MFA setup after login (e.g., "720h").
	// +optional
	MfaInitSkipLifetime string `json:"mfaInitSkipLifetime,omitempty"`

	// MultiFactorCheckLifetime is the duration before MFA re-verification (e.g., "12h").
	// +optional
	MultiFactorCheckLifetime string `json:"multiFactorCheckLifetime,omitempty"`

	// SecondFactorCheckLifetime is the duration before second factor re-verification (e.g., "12h").
	// +optional
	SecondFactorCheckLifetime string `json:"secondFactorCheckLifetime,omitempty"`

	// Idps is the list of identity providers allowed for login at the instance level.
	// +optional
	Idps []IdpReference `json:"idps,omitempty"`
}

// DefaultLoginPolicyStatus defines the observed state of DefaultLoginPolicy.
type DefaultLoginPolicyStatus struct {
	// Ready indicates whether the DefaultLoginPolicy is successfully synced.
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

// DefaultLoginPolicy is the Schema for the defaultloginpolicies API.
// It manages the instance-level default login policy (IAM_OWNER, Admin API).
// Singleton per instance: reconcile reads the current default, diffs all fields, updates on drift.
type DefaultLoginPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultLoginPolicySpec   `json:"spec,omitempty"`
	Status DefaultLoginPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DefaultLoginPolicyList contains a list of DefaultLoginPolicy.
type DefaultLoginPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultLoginPolicy `json:"items"`
}
