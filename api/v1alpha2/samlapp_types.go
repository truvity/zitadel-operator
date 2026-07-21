package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SAMLAppSpec defines the desired state of SAMLApp.
type SAMLAppSpec struct {
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

	// MetadataXml is the SAML SP metadata XML.
	// Mutually exclusive with MetadataUrl.
	// +optional
	MetadataXml string `json:"metadataXml,omitempty"`

	// MetadataUrl is the URL where the SAML SP metadata can be fetched.
	// Mutually exclusive with MetadataXml.
	// +optional
	MetadataUrl string `json:"metadataUrl,omitempty"`
}

// SAMLAppStatus defines the observed state of SAMLApp.
type SAMLAppStatus struct {
	// ApplicationId is the Zitadel application ID.
	ApplicationId string `json:"applicationId,omitempty"`

	// ProjectId is the resolved project ID this app belongs to.
	ProjectId string `json:"projectId,omitempty"`

	// OrganizationId is the resolved organization ID (inherited from project).
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the SAMLApp is successfully synced.
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
// +kubebuilder:printcolumn:name="AppID",type=string,JSONPath=`.status.applicationId`
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.status.projectId`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// SAMLApp is the Schema for the samlapps API.
type SAMLApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SAMLAppSpec   `json:"spec,omitempty"`
	Status SAMLAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SAMLAppList contains a list of SAMLApp.
type SAMLAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SAMLApp `json:"items"`
}

// DisplayName returns the app display name for Zitadel.
// Falls back to the Kubernetes resource name if spec.name is empty.
func (a *SAMLApp) DisplayName() string {
	if a.Spec.Name != "" {
		return a.Spec.Name
	}
	return a.Name
}
