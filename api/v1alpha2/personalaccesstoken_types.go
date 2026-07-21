package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PersonalAccessTokenSpec defines the desired state of PersonalAccessToken.
type PersonalAccessTokenSpec struct {
	// Instance optionally pins this resource to one operator's instance
	// identity — the operator config's instanceAlias, defaulting to its
	// domain (v0.18 dual-serving). When set to another identity, the CR is
	// ignored entirely so the owning operator can manage it. When empty
	// while the namespace is served by two operators, both fail closed with
	// an AmbiguousInstance condition.
	// +optional
	Instance string `json:"instance,omitempty"`

	// OrganizationRef references an Organization CR managed by this operator.
	// Mutually exclusive with OrganizationId.
	// +optional
	OrganizationRef *ResourceRef `json:"organizationRef,omitempty"`

	// OrganizationId references a pre-existing Zitadel organization by raw ID.
	// Mutually exclusive with OrganizationRef.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	// UserRef references a MachineUser CR managed by this operator.
	// Mutually exclusive with UserId.
	// +optional
	UserRef *ResourceRef `json:"userRef,omitempty"`

	// UserId references a pre-existing Zitadel user by raw ID.
	// Mutually exclusive with UserRef.
	// +optional
	UserId string `json:"userId,omitempty"`

	// ExpirationDate is the optional expiration timestamp for the token.
	// If not set, a 10-year expiration is used.
	// +optional
	ExpirationDate *metav1.Time `json:"expirationDate,omitempty"`

	// TokenSecretRef references the Secret where the PAT will be stored.
	TokenSecretRef TokenSecretRefSpec `json:"tokenSecretRef"`
}

// TokenSecretRefSpec references a Secret where the personal access token will be stored.
type TokenSecretRefSpec struct {
	// Name is the name of the Secret.
	Name string `json:"name"`

	// Key is the data key within the Secret. Default: "token"
	// +optional
	Key string `json:"key,omitempty"`
}

// PersonalAccessTokenStatus defines the observed state of PersonalAccessToken.
type PersonalAccessTokenStatus struct {
	// TokenId is the Zitadel personal access token ID.
	TokenId string `json:"tokenId,omitempty"`

	// UserId is the resolved user ID.
	UserId string `json:"userId,omitempty"`

	// OrganizationId is the resolved organization ID.
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the PersonalAccessToken is successfully synced.
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
// +kubebuilder:printcolumn:name="TokenID",type=string,JSONPath=`.status.tokenId`
// +kubebuilder:printcolumn:name="UserID",type=string,JSONPath=`.status.userId`
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
