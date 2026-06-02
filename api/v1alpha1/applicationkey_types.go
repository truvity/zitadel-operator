package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplicationKeySpec defines the desired state of ApplicationKey.
type ApplicationKeySpec struct {
	// AppId is the Zitadel application ID to create the key for.
	AppId string `json:"appId"`

	// ProjectRef is the name of the Zitadel Project CR this key belongs to.
	ProjectRef string `json:"projectRef"`

	// ExpirationDays is the number of days until the key expires.
	// Defaults to 3650 (10 years) if not set.
	// +optional
	ExpirationDays int `json:"expirationDays,omitempty"`

	// KeySecretRef references the Secret where the generated key JSON will be stored.
	KeySecretRef SecretRefSpec `json:"keySecretRef"`
}

// ApplicationKeyStatus defines the observed state of ApplicationKey.
type ApplicationKeyStatus struct {
	// KeyId is the Zitadel application key ID.
	KeyId string `json:"keyId,omitempty"`

	// Ready indicates whether the ApplicationKey is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AppId",type=string,JSONPath=`.spec.appId`
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.projectRef`
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
