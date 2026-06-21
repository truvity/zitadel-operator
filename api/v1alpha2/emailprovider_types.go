package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EmailProviderSpec defines the desired state of EmailProvider.
// Discriminated type: exactly one of Smtp or Http must be set.
type EmailProviderSpec struct {
	// Smtp configures an SMTP email provider.
	// Mutually exclusive with Http.
	// +optional
	Smtp *SmtpEmailProvider `json:"smtp,omitempty"`

	// Http configures an HTTP webhook email provider.
	// Mutually exclusive with Smtp.
	// +optional
	Http *HttpEmailProvider `json:"http,omitempty"`
}

// SmtpEmailProvider defines SMTP configuration for email delivery.
type SmtpEmailProvider struct {
	// Description is a human-readable description for this SMTP provider.
	// +optional
	Description string `json:"description,omitempty"`

	// SenderAddress is the email address used as the sender (From header).
	SenderAddress string `json:"senderAddress"`

	// SenderName is the display name of the sender.
	// +optional
	SenderName string `json:"senderName,omitempty"`

	// ReplyToAddress is the reply-to email address.
	// +optional
	ReplyToAddress string `json:"replyToAddress,omitempty"`

	// Tls enables TLS for the SMTP connection.
	// +optional
	Tls bool `json:"tls,omitempty"`

	// Host is the SMTP server hostname.
	Host string `json:"host"`

	// User is the SMTP authentication username.
	// +optional
	User string `json:"user,omitempty"`

	// PasswordSecretRef references the Secret containing the SMTP password.
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
}

// HttpEmailProvider defines HTTP webhook configuration for email delivery.
type HttpEmailProvider struct {
	// Endpoint is the HTTP(S) URL that receives email notifications.
	Endpoint string `json:"endpoint"`
}

// EmailProviderStatus defines the observed state of EmailProvider.
type EmailProviderStatus struct {
	// ProviderId is the Zitadel email provider ID.
	ProviderId string `json:"providerId,omitempty"`

	// Ready indicates whether the EmailProvider is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ProviderID",type=string,JSONPath=`.status.providerId`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// EmailProvider is the Schema for the emailproviders API.
// It manages an instance-level email provider (IAM_OWNER, Admin API).
// Discriminated type: smtp or http.
type EmailProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EmailProviderSpec   `json:"spec,omitempty"`
	Status EmailProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EmailProviderList contains a list of EmailProvider.
type EmailProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EmailProvider `json:"items"`
}
