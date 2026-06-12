package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIAppSpec defines the desired state of APIApp.
type APIAppSpec struct {
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

	// AuthMethod is the API authentication method.
	// +kubebuilder:validation:Enum=basic;private_key_jwt
	AuthMethod string `json:"authMethod"`

	// SecretRef references the Secret where the client credentials will be stored.
	SecretRef SecretRefSpec `json:"secretRef"`
}

// APIAppStatus defines the observed state of APIApp.
type APIAppStatus struct {
	// ApplicationId is the Zitadel application ID.
	ApplicationId string `json:"applicationId,omitempty"`

	// ClientId is the API client ID assigned by Zitadel.
	ClientId string `json:"clientId,omitempty"`

	// ProjectId is the resolved project ID this app belongs to.
	ProjectId string `json:"projectId,omitempty"`

	// OrganizationId is the resolved organization ID (inherited from project).
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the APIApp is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ClientID",type=string,JSONPath=`.status.clientId`
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.status.projectId`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// APIApp is the Schema for the apiapps API.
type APIApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIAppSpec   `json:"spec,omitempty"`
	Status APIAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// APIAppList contains a list of APIApp.
type APIAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIApp `json:"items"`
}

// DisplayName returns the app display name for Zitadel.
// Falls back to the Kubernetes resource name if spec.name is empty.
func (a *APIApp) DisplayName() string {
	if a.Spec.Name != "" {
		return a.Spec.Name
	}
	return a.Name
}
