package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ActionTrigger defines a flow and trigger type binding for an Action.
type ActionTrigger struct {
	// FlowType is the Zitadel flow type (e.g., "1" for Complement Token).
	FlowType string `json:"flowType"`

	// TriggerType is the Zitadel trigger type (e.g., "1" for Pre access token creation).
	TriggerType string `json:"triggerType"`
}

// ActionSpec defines the desired state of Action.
type ActionSpec struct {
	// Script is the JavaScript source code of the action.
	Script string `json:"script"`

	// AllowedToFail indicates whether the action is allowed to fail without blocking the flow.
	// +optional
	AllowedToFail bool `json:"allowedToFail,omitempty"`

	// Triggers defines the flow + trigger type bindings for this action.
	// +optional
	Triggers []ActionTrigger `json:"triggers,omitempty"`

	// Timeout is the action timeout in seconds. Default: 10.
	// +optional
	Timeout int `json:"timeout,omitempty"`
}

// ActionStatus defines the observed state of Action.
type ActionStatus struct {
	// ActionId is the Zitadel action ID.
	ActionId string `json:"actionId,omitempty"`

	// Ready indicates whether the Action is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// Action is the Schema for the actions API.
type Action struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ActionSpec   `json:"spec,omitempty"`
	Status ActionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ActionList contains a list of Action.
type ActionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Action `json:"items"`
}
