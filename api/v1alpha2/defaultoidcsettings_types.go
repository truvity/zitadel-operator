package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultOIDCSettingsSpec defines the desired state of DefaultOIDCSettings.
type DefaultOIDCSettingsSpec struct {
	OIDCSettingsFields `json:",inline"`
}

// DefaultOIDCSettingsStatus defines the observed state of DefaultOIDCSettings.
type DefaultOIDCSettingsStatus struct {
	// Ready indicates whether the DefaultOIDCSettings is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// DefaultOIDCSettings is the Schema for the defaultoidcsettings API.
// It manages the instance-level OIDC settings (IAM_OWNER, Admin API). Singleton.
type DefaultOIDCSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultOIDCSettingsSpec   `json:"spec,omitempty"`
	Status DefaultOIDCSettingsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DefaultOIDCSettingsList contains a list of DefaultOIDCSettings.
type DefaultOIDCSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultOIDCSettings `json:"items"`
}
