package v1alpha1

// SecretRefSpec references a Kubernetes Secret by name in the same namespace.
type SecretRefSpec struct {
	// Name is the name of the Secret.
	Name string `json:"name"`
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
