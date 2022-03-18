package v1

import (
	"context"
)

// ComposableCache caches objects that have been read so far in a reconcile cycle
type ComposableCache struct {
	objects map[string]interface{}
}

type toumbstone struct {
	err error
}

// ResolveObject interface
type ResolveObject interface {
	// ResolveObject resolves object references. It uses a context for cancellation.
	ResolveObject(ctx context.Context, in, out interface{}) error
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
