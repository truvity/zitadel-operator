package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LockoutPolicySpec defines the desired state of LockoutPolicy.
type LockoutPolicySpec struct {
	// MaxPasswordAttempts is the maximum number of failed password attempts before lockout.
	MaxPasswordAttempts int `json:"maxPasswordAttempts"`

	// MaxOtpAttempts is the maximum number of failed OTP attempts before lockout.
	// +optional
	MaxOtpAttempts int `json:"maxOtpAttempts,omitempty"`

	// OrganizationId is the Zitadel organization ID this policy applies to.
	// If empty, applies to the instance level.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`
}

// LockoutPolicyStatus defines the observed state of LockoutPolicy.
type LockoutPolicyStatus struct {
	// Ready indicates whether the LockoutPolicy is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="MaxPasswordAttempts",type=integer,JSONPath=`.spec.maxPasswordAttempts`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// LockoutPolicy is the Schema for the lockoutpolicies API.
type LockoutPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LockoutPolicySpec   `json:"spec,omitempty"`
	Status LockoutPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LockoutPolicyList contains a list of LockoutPolicy.
type LockoutPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LockoutPolicy `json:"items"`
}
