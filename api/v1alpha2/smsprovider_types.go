package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SmsProviderSpec defines the desired state of SmsProvider.
// Discriminated type: exactly one of Twilio or Http must be set.
type SmsProviderSpec struct {
	// Twilio configures a Twilio SMS provider.
	// Mutually exclusive with Http.
	// +optional
	Twilio *TwilioSmsProvider `json:"twilio,omitempty"`

	// Http configures an HTTP webhook SMS provider.
	// Mutually exclusive with Twilio.
	// +optional
	Http *HttpSmsProvider `json:"http,omitempty"`
}

// TwilioSmsProvider defines Twilio SMS configuration.
type TwilioSmsProvider struct {
	// SID is the Twilio account SID.
	SID string `json:"sid"`

	// TokenSecretRef references the Secret containing the Twilio auth token.
	TokenSecretRef SecretKeyRef `json:"tokenSecretRef"`

	// SenderNumber is the Twilio phone number to send from.
	SenderNumber string `json:"senderNumber"`

	// Description is a human-readable description.
	// +optional
	Description string `json:"description,omitempty"`
}

// HttpSmsProvider defines HTTP webhook SMS configuration.
type HttpSmsProvider struct {
	// Endpoint is the HTTP(S) URL that receives SMS notifications.
	Endpoint string `json:"endpoint"`

	// Description is a human-readable description.
	// +optional
	Description string `json:"description,omitempty"`
}

// SmsProviderStatus defines the observed state of SmsProvider.
type SmsProviderStatus struct {
	// ProviderId is the Zitadel SMS provider ID.
	ProviderId string `json:"providerId,omitempty"`

	// Ready indicates whether the SmsProvider is successfully synced.
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
// +kubebuilder:printcolumn:name="ProviderID",type=string,JSONPath=`.status.providerId`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// SmsProvider is the Schema for the smsproviders API.
// It manages an instance-level SMS provider (Admin API).
// Discriminated type: twilio or http.
type SmsProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SmsProviderSpec   `json:"spec,omitempty"`
	Status SmsProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SmsProviderList contains a list of SmsProvider.
type SmsProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SmsProvider `json:"items"`
}
