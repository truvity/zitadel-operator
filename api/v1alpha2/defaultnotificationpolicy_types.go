package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultNotificationPolicySpec defines the desired state of DefaultNotificationPolicy.
type DefaultNotificationPolicySpec struct {
	NotificationPolicyFields `json:",inline"`
}

// DefaultNotificationPolicyStatus defines the observed state of DefaultNotificationPolicy.
type DefaultNotificationPolicyStatus struct {
	// Ready indicates whether the DefaultNotificationPolicy is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// DefaultNotificationPolicy is the Schema for the defaultnotificationpolicies API.
// It manages the instance-level default notification policy (IAM_OWNER, Admin API). Singleton.
type DefaultNotificationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultNotificationPolicySpec   `json:"spec,omitempty"`
	Status DefaultNotificationPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DefaultNotificationPolicyList contains a list of DefaultNotificationPolicy.
type DefaultNotificationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultNotificationPolicy `json:"items"`
}
