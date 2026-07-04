package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ActionTargetType defines the type of target and how the response is treated.
// +kubebuilder:validation:Enum=restCall;restWebhook;restAsync
type ActionTargetType string

const (
	// ActionTargetTypeRestCall makes a POST request and reads the response body.
	// This is required when Zitadel needs to read the response (e.g., append_claims).
	ActionTargetTypeRestCall ActionTargetType = "restCall"

	// ActionTargetTypeRestWebhook makes a POST request but ignores the response body.
	// Only the status code is checked.
	ActionTargetTypeRestWebhook ActionTargetType = "restWebhook"

	// ActionTargetTypeRestAsync makes an asynchronous POST request.
	// Zitadel does not wait for the response. Typically used for event executions.
	ActionTargetTypeRestAsync ActionTargetType = "restAsync"
)

// ActionTargetPayloadType defines how the payload is formatted and secured.
// +kubebuilder:validation:Enum=json;jwt;jwe
type ActionTargetPayloadType string

const (
	// ActionTargetPayloadTypeJSON sends the payload as JSON with X-ZITADEL-Signature header.
	ActionTargetPayloadTypeJSON ActionTargetPayloadType = "json"

	// ActionTargetPayloadTypeJWT sends the payload as a signed JWT.
	// The receiver can verify authenticity and integrity using the signing key.
	ActionTargetPayloadTypeJWT ActionTargetPayloadType = "jwt"

	// ActionTargetPayloadTypeJWE sends the payload as an encrypted JWT.
	ActionTargetPayloadTypeJWE ActionTargetPayloadType = "jwe"
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

	// TargetType defines the type of target and how Zitadel treats the response.
	// - restCall: reads the response body (required for append_claims)
	// - restWebhook: only checks status code, ignores body
	// - restAsync: fire-and-forget, does not wait for response
	// Default: restCall
	// +optional
	// +kubebuilder:default=restCall
	TargetType ActionTargetType `json:"targetType,omitempty"`

	// PayloadType defines how the payload is formatted and secured.
	// - json: JSON body with X-ZITADEL-Signature header (default)
	// - jwt: signed JWT body (receiver verifies via JWKS)
	// - jwe: encrypted JWT body
	// Default: json
	// +optional
	// +kubebuilder:default=json
	PayloadType ActionTargetPayloadType `json:"payloadType,omitempty"`
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
