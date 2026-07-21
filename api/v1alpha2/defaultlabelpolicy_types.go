package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultLabelPolicySpec defines the desired state of DefaultLabelPolicy.
type DefaultLabelPolicySpec struct {
	LabelPolicyFields `json:",inline"`
}

// DefaultLabelPolicyStatus defines the observed state of DefaultLabelPolicy.
type DefaultLabelPolicyStatus struct {
	// Ready indicates whether the DefaultLabelPolicy is successfully synced.
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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// DefaultLabelPolicy is the Schema for the defaultlabelpolicies API.
// It manages the instance-level default label/branding policy (IAM_OWNER, Admin API). Singleton.
type DefaultLabelPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultLabelPolicySpec   `json:"spec,omitempty"`
	Status DefaultLabelPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DefaultLabelPolicyList contains a list of DefaultLabelPolicy.
type DefaultLabelPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultLabelPolicy `json:"items"`
}
