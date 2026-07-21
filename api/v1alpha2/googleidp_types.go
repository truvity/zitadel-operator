package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretKeyRef references a key within a Kubernetes Secret.
type SecretKeyRef struct {
	// Name is the name of the Secret.
	Name string `json:"name"`

	// Key is the key within the Secret's data. Default: "clientSecret"
	// +optional
	Key string `json:"key,omitempty"`
}

// GoogleIdPSpec defines the desired state of GoogleIdP.
type GoogleIdPSpec struct {
	// Name is the display name of the Google identity provider in Zitadel.
	Name string `json:"name"`

	// ClientId is the Google OAuth2 client ID.
	ClientId string `json:"clientId"`

	// ClientSecretRef references the Secret containing the Google OAuth2 client secret.
	ClientSecretRef SecretKeyRef `json:"clientSecretRef"`

	// Scopes are the OAuth2 scopes to request from Google.
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// IsLinkingAllowed allows linking existing Zitadel accounts with Google accounts.
	// +optional
	IsLinkingAllowed bool `json:"isLinkingAllowed,omitempty"`

	// IsCreationAllowed allows automatic user creation on first Google login.
	// +optional
	IsCreationAllowed bool `json:"isCreationAllowed,omitempty"`

	// IsAutoCreation enables automatic user creation (same as IsCreationAllowed in Zitadel API).
	// +optional
	IsAutoCreation bool `json:"isAutoCreation,omitempty"`

	// IsAutoUpdate enables automatic user profile update on Google login.
	// +optional
	IsAutoUpdate bool `json:"isAutoUpdate,omitempty"`
}

// GoogleIdPStatus defines the observed state of GoogleIdP.
type GoogleIdPStatus struct {
	// IdpID is the Zitadel identity provider ID for this Google IdP.
	// Exposed so that DefaultLoginPolicy.idps[].idpRef can resolve it.
	IdpID string `json:"idpID,omitempty"`

	// Ready indicates whether the GoogleIdP is successfully synced.
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
// +kubebuilder:printcolumn:name="IdpID",type=string,JSONPath=`.status.idpID`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// GoogleIdP is the Schema for the googleidps API.
// It manages an instance-scoped Google identity provider (IAM_OWNER, Admin API).
// Uses Admin API AddGoogleProvider/UpdateGoogleProvider.
type GoogleIdP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GoogleIdPSpec   `json:"spec,omitempty"`
	Status GoogleIdPStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GoogleIdPList contains a list of GoogleIdP.
type GoogleIdPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GoogleIdP `json:"items"`
}

// DisplayName returns the IdP display name for Zitadel.
func (g *GoogleIdP) DisplayName() string {
	if g.Spec.Name != "" {
		return g.Spec.Name
	}
	return g.Name
}
