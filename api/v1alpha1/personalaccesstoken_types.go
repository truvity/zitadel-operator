package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PersonalAccessTokenSpec defines the desired state of PersonalAccessToken.
type PersonalAccessTokenSpec struct {
	// UserId is the Zitadel user ID to create the token for.
	UserId string `json:"userId"`

	// ExpirationDays is the number of days until the token expires.
	// Defaults to 365 (1 year) if not set.
	// +optional
	ExpirationDays int `json:"expirationDays,omitempty"`

	// TokenSecretRef references the Secret where the generated token will be stored.
	TokenSecretRef SecretRefSpec `json:"tokenSecretRef"`
}

// PersonalAccessTokenStatus defines the observed state of PersonalAccessToken.
type PersonalAccessTokenStatus struct {
	// TokenId is the Zitadel personal access token ID.
	TokenId string `json:"tokenId,omitempty"`

	// Ready indicates whether the PersonalAccessToken is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="User",type=string,JSONPath=`.spec.userId`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// PersonalAccessToken is the Schema for the personalaccesstokens API.
type PersonalAccessToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PersonalAccessTokenSpec   `json:"spec,omitempty"`
	Status PersonalAccessTokenStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PersonalAccessTokenList contains a list of PersonalAccessToken.
type PersonalAccessTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PersonalAccessToken `json:"items"`
}
