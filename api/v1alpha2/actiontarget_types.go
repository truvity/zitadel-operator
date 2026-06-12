package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ActionTargetSpec defines the desired state of ActionTarget.
type ActionTargetSpec struct {
	// Name is the display name of the target in Zitadel.
	// If empty, the Kubernetes resource name is used.
	// +optional
	Name string `json:"name,omitempty"`

	// Endpoint is the HTTP(S) URL the target will call.
	Endpoint string `json:"endpoint"`

	// Timeout is the request timeout duration (e.g., "10s", "30s").
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// InterruptOnError determines whether the action flow stops if this target fails.
	// +optional
	InterruptOnError bool `json:"interruptOnError,omitempty"`
}

// ActionTargetStatus defines the observed state of ActionTarget.
type ActionTargetStatus struct {
	// TargetId is the Zitadel target ID.
	TargetId string `json:"targetId,omitempty"`

	// Ready indicates whether the ActionTarget is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TargetID",type=string,JSONPath=`.status.targetId`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.endpoint`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// ActionTarget is the Schema for the actiontargets API.
type ActionTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ActionTargetSpec   `json:"spec,omitempty"`
	Status ActionTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ActionTargetList contains a list of ActionTarget.
type ActionTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ActionTarget `json:"items"`
}

// DisplayName returns the target display name for Zitadel.
func (a *ActionTarget) DisplayName() string {
	if a.Spec.Name != "" {
		return a.Spec.Name
	}
	return a.Name
}
