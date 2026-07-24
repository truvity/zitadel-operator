package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OIDCAppSpec defines the desired state of OIDCApp.
type OIDCAppSpec struct {
	// Instance optionally pins this resource to one operator's instance
	// identity — the operator config's instanceAlias, defaulting to its
	// domain (v0.18 dual-serving). When set to another identity, the CR is
	// ignored entirely so the owning operator can manage it. When empty
	// while the namespace is served by two operators, both fail closed with
	// an AmbiguousInstance condition.
	// +optional
	Instance string `json:"instance,omitempty"`

	// ProjectRef references a Project CR managed by this operator.
	// Mutually exclusive with ProjectId.
	// +optional
	ProjectRef *ResourceRef `json:"projectRef,omitempty"`

	// ProjectId references a pre-existing Zitadel project by raw ID.
	// Mutually exclusive with ProjectRef.
	// +optional
	ProjectId string `json:"projectId,omitempty"`

	// Name is the display name of the application in Zitadel.
	// If empty, the Kubernetes resource name is used.
	// +optional
	Name string `json:"name,omitempty"`

	// Type is the OIDC application type.
	// +kubebuilder:validation:Enum=confidential;public
	Type string `json:"type"`

	// AuthMethod is the authentication method.
	// +kubebuilder:validation:Enum=basic;none
	AuthMethod string `json:"authMethod"`

	// RedirectUris is the exact, ordered list of allowed OAuth2/OIDC redirect
	// URIs. The operator is the single writer: server-side drift (added or
	// removed entries) is reverted to this list on every reconcile. Zitadel
	// requires https:// URIs unless the app has dev mode enabled.
	RedirectUris []string `json:"redirectUris"`

	// PostLogoutRedirectUris is the list of allowed URIs to return users to
	// after logout (RP-initiated logout). Drift-corrected like RedirectUris.
	// +optional
	PostLogoutRedirectUris []string `json:"postLogoutRedirectUris,omitempty"`

	// AccessTokenType selects the access token format: "bearer" (opaque,
	// default) or "jwt" (self-contained, locally verifiable).
	// +kubebuilder:validation:Enum=bearer;jwt
	// +optional
	AccessTokenType string `json:"accessTokenType,omitempty"`

	// AccessTokenRoleAssertion includes the user's project role claims in
	// the access token.
	// +optional
	AccessTokenRoleAssertion bool `json:"accessTokenRoleAssertion,omitempty"`

	// IdTokenRoleAssertion includes the user's project role claims in the ID token.
	// +optional
	IdTokenRoleAssertion bool `json:"idTokenRoleAssertion,omitempty"`

	// IdTokenUserinfoAssertion asserts the userinfo claims (including
	// action-appended claims such as the rbac-mapper's groups) into the ID
	// token at issuance. Required for consumers that read groups from the ID
	// token (ArgoCD, kubelogin-style flows).
	// +optional
	IdTokenUserinfoAssertion bool `json:"idTokenUserinfoAssertion,omitempty"`

	// SecretRef references the Secret where the client credentials will be stored.
	SecretRef SecretRefSpec `json:"secretRef"`
}

// OIDCAppStatus defines the observed state of OIDCApp.
type OIDCAppStatus struct {
	// ApplicationId is the Zitadel application ID.
	ApplicationId string `json:"applicationId,omitempty"`

	// ClientId is the OIDC client ID assigned by Zitadel.
	ClientId string `json:"clientId,omitempty"`

	// ProjectId is the resolved project ID this app belongs to.
	ProjectId string `json:"projectId,omitempty"`

	// OrganizationId is the resolved organization ID (inherited from project).
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the OIDCApp is successfully synced.
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
// +kubebuilder:printcolumn:name="ClientID",type=string,JSONPath=`.status.clientId`
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.status.projectId`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// OIDCApp is the Schema for the oidcapps API.
type OIDCApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OIDCAppSpec   `json:"spec,omitempty"`
	Status OIDCAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OIDCAppList contains a list of OIDCApp.
type OIDCAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OIDCApp `json:"items"`
}

// DisplayName returns the app display name for Zitadel.
// Falls back to the Kubernetes resource name if spec.name is empty.
func (a *OIDCApp) DisplayName() string {
	if a.Spec.Name != "" {
		return a.Spec.Name
	}
	return a.Name
}
