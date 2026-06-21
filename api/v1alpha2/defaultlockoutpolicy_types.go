package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultLockoutPolicySpec defines the desired state of DefaultLockoutPolicy.
type DefaultLockoutPolicySpec struct {
	LockoutPolicyFields `json:",inline"`
}

// DefaultLockoutPolicyStatus defines the observed state of DefaultLockoutPolicy.
type DefaultLockoutPolicyStatus struct {
	// Ready indicates whether the DefaultLockoutPolicy is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// DefaultLockoutPolicy is the Schema for the defaultlockoutpolicies API.
// It manages the instance-level default lockout policy (IAM_OWNER, Admin API). Singleton.
type DefaultLockoutPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultLockoutPolicySpec   `json:"spec,omitempty"`
	Status DefaultLockoutPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DefaultLockoutPolicyList contains a list of DefaultLockoutPolicy.
type DefaultLockoutPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultLockoutPolicy `json:"items"`
}
