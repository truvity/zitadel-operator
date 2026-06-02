package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PasswordComplexityPolicySpec defines the desired state of PasswordComplexityPolicy.
type PasswordComplexityPolicySpec struct {
	// MinLength is the minimum password length required.
	MinLength int `json:"minLength"`

	// HasUppercase requires at least one uppercase character.
	// +optional
	HasUppercase bool `json:"hasUppercase,omitempty"`

	// HasLowercase requires at least one lowercase character.
	// +optional
	HasLowercase bool `json:"hasLowercase,omitempty"`

	// HasNumber requires at least one numeric character.
	// +optional
	HasNumber bool `json:"hasNumber,omitempty"`

	// HasSymbol requires at least one special character.
	// +optional
	HasSymbol bool `json:"hasSymbol,omitempty"`

	// OrganizationId is the Zitadel organization ID this policy applies to.
	// If empty, applies to the instance level.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`
}

// PasswordComplexityPolicyStatus defines the observed state of PasswordComplexityPolicy.
type PasswordComplexityPolicyStatus struct {
	// Ready indicates whether the PasswordComplexityPolicy is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="MinLength",type=integer,JSONPath=`.spec.minLength`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// PasswordComplexityPolicy is the Schema for the passwordcomplexitypolicies API.
type PasswordComplexityPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PasswordComplexityPolicySpec   `json:"spec,omitempty"`
	Status PasswordComplexityPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PasswordComplexityPolicyList contains a list of PasswordComplexityPolicy.
type PasswordComplexityPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PasswordComplexityPolicy `json:"items"`
}
