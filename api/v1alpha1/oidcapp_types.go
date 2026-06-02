package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OIDCAppSpec defines the desired state of OIDCApp.
type OIDCAppSpec struct {
	// Project is the name of the Zitadel project this app belongs to.
	Project string `json:"project"`

	// Type is the OIDC application type (confidential or public).
	// +kubebuilder:validation:Enum=confidential;public
	Type string `json:"type"`

	// AuthMethod is the authentication method (basic or none).
	// +kubebuilder:validation:Enum=basic;none
	AuthMethod string `json:"authMethod"`

	// RedirectUris is the list of allowed redirect URIs.
	RedirectUris []string `json:"redirectUris"`

	// SecretRef references the Secret where the client secret will be stored.
	SecretRef SecretRefSpec `json:"secretRef"`
}

// OIDCAppStatus defines the observed state of OIDCApp.
type OIDCAppStatus struct {
	// ClientId is the OIDC client ID assigned by Zitadel.
	ClientId string `json:"clientId,omitempty"`

	// Ready indicates whether the OIDCApp is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.project`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
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
