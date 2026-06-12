package v1alpha2

// ResourceRef references a Kubernetes resource by name and optional namespace.
// If namespace is omitted, defaults to the same namespace as the referencing resource.
type ResourceRef struct {
	// Name is the name of the referenced resource.
	Name string `json:"name"`

	// Namespace is the namespace of the referenced resource.
	// If empty, defaults to the same namespace as the referencing resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// SecretRefSpec references a Kubernetes Secret where credentials will be stored.
type SecretRefSpec struct {
	// Name is the name of the Secret.
	Name string `json:"name"`

	// Keys customizes the secret key names for clientId and clientSecret.
	// Defaults: clientId → "client_id", clientSecret → "client_secret"
	// +optional
	Keys *SecretKeys `json:"keys,omitempty"`

	// ExtraData adds static key-value pairs to the generated secret.
	// +optional
	ExtraData map[string]string `json:"extraData,omitempty"`
}

// SecretKeys customizes the key names used in the generated Kubernetes Secret.
type SecretKeys struct {
	// ClientId is the key name for the client ID in the generated secret.
	// Default: "client_id"
	// +optional
	ClientId string `json:"clientId,omitempty"`

	// ClientSecret is the key name for the client secret in the generated secret.
	// Default: "client_secret"
	// +optional
	ClientSecret string `json:"clientSecret,omitempty"`
}

// MachineKeySecretRef references a Secret where the machine user key JSON will be stored.
type MachineKeySecretRef struct {
	// Name is the name of the Secret.
	Name string `json:"name"`

	// Key is the data key within the Secret. Default: "key.json"
	// +optional
	Key string `json:"key,omitempty"`
}
