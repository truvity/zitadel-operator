package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LoginPolicySpec defines the desired state of LoginPolicy (org-scoped).
type LoginPolicySpec struct {
	// OrganizationRef references an Organization CR managed by this operator.
	// Mutually exclusive with OrganizationId.
	// +optional
	OrganizationRef *ResourceRef `json:"organizationRef,omitempty"`

	// OrganizationId references a pre-existing Zitadel organization by raw ID.
	// Mutually exclusive with OrganizationRef.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	LoginPolicyFields `json:",inline"`

	// DisableLoginWithEmail disables login using email addresses.
	// +optional
	DisableLoginWithEmail *bool `json:"disableLoginWithEmail,omitempty"`

	// DisableLoginWithPhone disables login using phone numbers.
	// +optional
	DisableLoginWithPhone *bool `json:"disableLoginWithPhone,omitempty"`

	// SecondFactors is the list of allowed second factor types.
	// +kubebuilder:validation:Items:Enum=otp;u2f;otp_email;otp_sms
	// +optional
	SecondFactors []string `json:"secondFactors,omitempty"`

	// MultiFactors is the list of allowed multi-factor types.
	// +kubebuilder:validation:Items:Enum=u2f_with_verification
	// +optional
	MultiFactors []string `json:"multiFactors,omitempty"`

	// Idps is the list of identity providers allowed for login at the org level.
	// +optional
	Idps []IdpReference `json:"idps,omitempty"`
}

// LoginPolicyStatus defines the observed state of LoginPolicy.
type LoginPolicyStatus struct {
	// Ready indicates whether the LoginPolicy is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// OrganizationId is the resolved organization ID.
	OrganizationId string `json:"organizationId,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// LoginPolicy is the Schema for the loginpolicies API.
// It manages an org-scoped login policy (ORG_OWNER, Management API).
type LoginPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoginPolicySpec   `json:"spec,omitempty"`
	Status LoginPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LoginPolicyList contains a list of LoginPolicy.
type LoginPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoginPolicy `json:"items"`
}
