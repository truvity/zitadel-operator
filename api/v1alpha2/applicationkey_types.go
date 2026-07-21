package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplicationKeySpec defines the desired state of ApplicationKey.
type ApplicationKeySpec struct {
	// ProjectRef references a Project CR managed by this operator.
	// Mutually exclusive with ProjectId.
	// +optional
	ProjectRef *ResourceRef `json:"projectRef,omitempty"`

	// ProjectId references a pre-existing Zitadel project by raw ID.
	// Mutually exclusive with ProjectRef.
	// +optional
	ProjectId string `json:"projectId,omitempty"`

	// AppRef references an OIDCApp or APIApp CR managed by this operator.
	// Mutually exclusive with AppId.
	// +optional
	AppRef *ResourceRef `json:"appRef,omitempty"`

	// AppId references a pre-existing Zitadel application by raw ID.
	// Mutually exclusive with AppRef.
	// +optional
	AppId string `json:"appId,omitempty"`

	// KeyType specifies the key format. Currently only JSON is supported.
	// +kubebuilder:validation:Enum=json
	// +kubebuilder:default=json
	// +optional
	KeyType string `json:"keyType,omitempty"`

	// ExpirationDate is the optional expiration timestamp for the key.
	// If not set, a 10-year expiration is used.
	// +optional
	ExpirationDate *metav1.Time `json:"expirationDate,omitempty"`

	// KeySecretRef references the Secret where the key JSON will be stored.
	KeySecretRef MachineKeySecretRef `json:"keySecretRef"`
}

// ApplicationKeyStatus defines the observed state of ApplicationKey.
type ApplicationKeyStatus struct {
	// KeyId is the Zitadel application key ID.
	KeyId string `json:"keyId,omitempty"`

	// ProjectId is the resolved project ID.
	ProjectId string `json:"projectId,omitempty"`

	// AppId is the resolved application ID.
	AppId string `json:"appId,omitempty"`

	// Ready indicates whether the ApplicationKey is successfully synced.
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
// +kubebuilder:printcolumn:name="KeyID",type=string,JSONPath=`.status.keyId`
// +kubebuilder:printcolumn:name="AppID",type=string,JSONPath=`.status.appId`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// ApplicationKey is the Schema for the applicationkeys API.
type ApplicationKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationKeySpec   `json:"spec,omitempty"`
	Status ApplicationKeyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationKeyList contains a list of ApplicationKey.
type ApplicationKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationKey `json:"items"`
}
