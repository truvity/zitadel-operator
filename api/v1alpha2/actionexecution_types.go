package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ActionExecutionSpec defines the desired state of ActionExecution.
type ActionExecutionSpec struct {
	// Condition defines when the execution triggers.
	Condition ActionCondition `json:"condition"`

	// Targets is the list of target references to invoke.
	Targets []ActionExecutionTarget `json:"targets"`
}

// ActionCondition defines the trigger condition for an execution.
type ActionCondition struct {
	// Function is a Zitadel function condition (e.g., "/zitadel.session.v2.SessionService/SetSession").
	// Mutually exclusive with Request and Event.
	// +optional
	Function string `json:"function,omitempty"`

	// Request is a Zitadel request condition (e.g., "/zitadel.user.v2.UserService/AddHumanUser").
	// Mutually exclusive with Function and Event.
	// +optional
	Request string `json:"request,omitempty"`

	// Event is a Zitadel event condition (e.g., "user.human.added").
	// Mutually exclusive with Function and Request.
	// +optional
	Event string `json:"event,omitempty"`
}

// ActionExecutionTarget references a target to invoke in the execution.
type ActionExecutionTarget struct {
	// TargetRef references an ActionTarget CR managed by this operator.
	// Mutually exclusive with TargetId.
	// +optional
	TargetRef *ResourceRef `json:"targetRef,omitempty"`

	// TargetId references a pre-existing Zitadel target by raw ID.
	// Mutually exclusive with TargetRef.
	// +optional
	TargetId string `json:"targetId,omitempty"`
}

// ActionExecutionStatus defines the observed state of ActionExecution.
type ActionExecutionStatus struct {
	// Ready indicates whether the ActionExecution is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// ActionExecution is the Schema for the actionexecutions API.
type ActionExecution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ActionExecutionSpec   `json:"spec,omitempty"`
	Status ActionExecutionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ActionExecutionList contains a list of ActionExecution.
type ActionExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ActionExecution `json:"items"`
}
