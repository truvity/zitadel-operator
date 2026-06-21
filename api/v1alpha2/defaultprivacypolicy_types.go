package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultPrivacyPolicySpec defines the desired state of DefaultPrivacyPolicy.
type DefaultPrivacyPolicySpec struct {
	PrivacyPolicyFields `json:",inline"`
}

// DefaultPrivacyPolicyStatus defines the observed state of DefaultPrivacyPolicy.
type DefaultPrivacyPolicyStatus struct {
	// Ready indicates whether the DefaultPrivacyPolicy is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// DefaultPrivacyPolicy is the Schema for the defaultprivacypolicies API.
// It manages the instance-level default privacy policy (IAM_OWNER, Admin API). Singleton.
type DefaultPrivacyPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultPrivacyPolicySpec   `json:"spec,omitempty"`
	Status DefaultPrivacyPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DefaultPrivacyPolicyList contains a list of DefaultPrivacyPolicy.
type DefaultPrivacyPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultPrivacyPolicy `json:"items"`
}
