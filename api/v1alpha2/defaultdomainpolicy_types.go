package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultDomainPolicySpec defines the desired state of DefaultDomainPolicy.
type DefaultDomainPolicySpec struct {
	// UserLoginMustBeDomain requires user login names to include the org domain.
	// +optional
	UserLoginMustBeDomain *bool `json:"userLoginMustBeDomain,omitempty"`

	// ValidateOrgDomains requires organization domains to be verified via DNS.
	// +optional
	ValidateOrgDomains *bool `json:"validateOrgDomains,omitempty"`

	// SmtpSenderAddressMatchesInstanceDomain requires SMTP sender to match the instance domain.
	// +optional
	SmtpSenderAddressMatchesInstanceDomain *bool `json:"smtpSenderAddressMatchesInstanceDomain,omitempty"`
}

// DefaultDomainPolicyStatus defines the observed state of DefaultDomainPolicy.
type DefaultDomainPolicyStatus struct {
	// Ready indicates whether the DefaultDomainPolicy is successfully synced.
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

// DefaultDomainPolicy is the Schema for the defaultdomainpolicies API.
// It manages the instance-level default domain policy (IAM_OWNER, Admin API).
// Singleton per instance: reconcile reads the current default, diffs all fields, updates on drift.
type DefaultDomainPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultDomainPolicySpec   `json:"spec,omitempty"`
	Status DefaultDomainPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DefaultDomainPolicyList contains a list of DefaultDomainPolicy.
type DefaultDomainPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultDomainPolicy `json:"items"`
}
