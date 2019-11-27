package v1

// ComposableCache caches objects that have been read so far in a reconcile cycle
type ComposableCache struct {
	objects map[string]interface{}
}

type toumbstone struct {
	err ComposableError
}

// ComposableError is the error type returned by the composable's Resolve function
type ComposableError struct {
	Error error
	// This indicates that the consuming Reconcile function should return this error
	ShouldBeReturned bool
}

// ComposableGetValueFrom specifies a reference to a Kubernetes object
// +kubebuilder:object:generate=true
type ComposableGetValueFrom struct {
	Kind               string   `json:"kind"`
	APIVersion         string   `json:"apiVersion,omitempty"`
	Name               string   `json:"name,omitempty"`
	Labels             []string `json:"labels,omitempty"`
	Namespace          string   `json:"namespace,omitempty"`
	Path               string   `json:"path"`
	FormatTransformers []string `json:"format-transformers,omitempty"`
}

// ObjectRef is the type that can be used for cross-resource references
// +kubebuilder:object:generate=true
type ObjectRef struct {
	GetValueFrom ComposableGetValueFrom `json:"getValueFrom"`
}
