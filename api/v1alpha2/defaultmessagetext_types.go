package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultMessageTextSpec defines the desired state of DefaultMessageText.
type DefaultMessageTextSpec struct {
	MessageTextFields `json:",inline"`
}

// DefaultMessageTextStatus defines the observed state of DefaultMessageText.
type DefaultMessageTextStatus struct {
	// Ready indicates whether the DefaultMessageText is successfully synced.
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
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Language",type=string,JSONPath=`.spec.language`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// DefaultMessageText is the Schema for the defaultmessagetexts API.
// It manages instance-level default message text (IAM_OWNER, Admin API). One CR per type+language combination.
type DefaultMessageText struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultMessageTextSpec   `json:"spec,omitempty"`
	Status DefaultMessageTextStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DefaultMessageTextList contains a list of DefaultMessageText.
type DefaultMessageTextList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultMessageText `json:"items"`
}
