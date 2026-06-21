package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultPasswordAgePolicySpec defines the desired state of DefaultPasswordAgePolicy.
type DefaultPasswordAgePolicySpec struct {
	PasswordAgePolicyFields `json:",inline"`
}

// DefaultPasswordAgePolicyStatus defines the observed state of DefaultPasswordAgePolicy.
type DefaultPasswordAgePolicyStatus struct {
	// Ready indicates whether the DefaultPasswordAgePolicy is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// DefaultPasswordAgePolicy is the Schema for the defaultpasswordagepolicies API.
// It manages the instance-level default password age policy (IAM_OWNER, Admin API). Singleton.
type DefaultPasswordAgePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultPasswordAgePolicySpec   `json:"spec,omitempty"`
	Status DefaultPasswordAgePolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DefaultPasswordAgePolicyList contains a list of DefaultPasswordAgePolicy.
type DefaultPasswordAgePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultPasswordAgePolicy `json:"items"`
}
