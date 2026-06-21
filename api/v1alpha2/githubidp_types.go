package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitHubIdPSpec defines the desired state of GitHubIdP.
type GitHubIdPSpec struct {
	// Name is the display name of the GitHub identity provider in Zitadel.
	Name string `json:"name"`

	// ClientId is the GitHub OAuth2 client ID.
	ClientId string `json:"clientId"`

	// ClientSecretRef references the Secret containing the GitHub OAuth2 client secret.
	ClientSecretRef SecretKeyRef `json:"clientSecretRef"`

	// Scopes are the OAuth2 scopes to request from GitHub.
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// IsLinkingAllowed allows linking existing Zitadel accounts with GitHub accounts.
	// +optional
	IsLinkingAllowed bool `json:"isLinkingAllowed,omitempty"`

	// IsCreationAllowed allows automatic user creation on first GitHub login.
	// +optional
	IsCreationAllowed bool `json:"isCreationAllowed,omitempty"`

	// IsAutoCreation enables automatic user creation (same as IsCreationAllowed in Zitadel API).
	// +optional
	IsAutoCreation bool `json:"isAutoCreation,omitempty"`

	// IsAutoUpdate enables automatic user profile update on GitHub login.
	// +optional
	IsAutoUpdate bool `json:"isAutoUpdate,omitempty"`
}

// GitHubIdPStatus defines the observed state of GitHubIdP.
type GitHubIdPStatus struct {
	// IdpID is the Zitadel identity provider ID for this GitHub IdP.
	IdpID string `json:"idpID,omitempty"`

	// Ready indicates whether the GitHubIdP is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="IdpID",type=string,JSONPath=`.status.idpID`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// GitHubIdP is the Schema for the githubidps API.
// It manages an instance-scoped GitHub identity provider (IAM_OWNER, Admin API).
// Uses Admin API AddGitHubProvider/UpdateGitHubProvider.
type GitHubIdP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubIdPSpec   `json:"spec,omitempty"`
	Status GitHubIdPStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitHubIdPList contains a list of GitHubIdP.
type GitHubIdPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubIdP `json:"items"`
}

// DisplayName returns the IdP display name for Zitadel.
func (g *GitHubIdP) DisplayName() string {
	if g.Spec.Name != "" {
		return g.Spec.Name
	}
	return g.Name
}
