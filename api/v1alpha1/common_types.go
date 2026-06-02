package v1alpha1

// SecretRefSpec references a Kubernetes Secret by name in the same namespace.
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

// SecretKeyRef references a specific key in a Kubernetes Secret.
type SecretKeyRef struct {
	// Name is the name of the Secret.
	Name string `json:"name"`

	// Namespace is the namespace of the Secret.
	Namespace string `json:"namespace"`

	// Key is the key within the Secret data.
	Key string `json:"key"`
}

// NamespacedRef references a resource by name and namespace.
type NamespacedRef struct {
	// Name is the name of the resource.
	Name string `json:"name"`

	// Namespace is the namespace of the resource.
	Namespace string `json:"namespace"`
}
