package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IdentityProviderSpec defines the desired state of IdentityProvider.
type IdentityProviderSpec struct {
	// OrganizationRef references an Organization CR managed by this operator.
	// Mutually exclusive with OrganizationId.
	// +optional
	OrganizationRef *ResourceRef `json:"organizationRef,omitempty"`

	// OrganizationId references a pre-existing Zitadel organization by raw ID.
	// Mutually exclusive with OrganizationRef.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	// Name is the display name for the identity provider.
	Name string `json:"name"`

	// Issuer is the OIDC issuer URL.
	Issuer string `json:"issuer"`

	// ClientId is the OIDC client ID.
	ClientId string `json:"clientId"`

	// ClientSecret is the OIDC client secret.
	ClientSecret string `json:"clientSecret"`

	// Scopes are the OIDC scopes to request.
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// IsAutoCreation enables automatic user creation on first login.
	// +optional
	IsAutoCreation bool `json:"isAutoCreation,omitempty"`

	// IsAutoUpdate enables automatic user profile update on login.
	// +optional
	IsAutoUpdate bool `json:"isAutoUpdate,omitempty"`

	// IsLinkingAllowed enables account linking.
	// +optional
	IsLinkingAllowed bool `json:"isLinkingAllowed,omitempty"`
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
// +kubebuilder:printcolumn:name="IdpID",type=string,JSONPath=`.status.idpId`
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
