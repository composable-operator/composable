package v1

type ComposableCache struct {
	objects map[string]interface{}
}

type toumbstone struct {
	err ComposableError
}

type ComposableError struct {
	Error error
	// TODO do we need this state separation
	IsPendable bool
	// if the error is retrievable the controller will return it to the manager, and teh last will recall Reconcile again
	IsRetrievable bool
}

//ComposableGetValueFrom is the struct for Composable getValueFrom
type composableGetValueFrom struct {
	Kind               string   `json:"kind"`
	APIVersion         string   `json:"apiVersion,omitempty"`
	Name               string   `json:"name,omitempty"`
	Labels             []string `json:"labels,omitempty"`
	Namespace          string   `json:"namespace,omitempty"`
	Path               string   `json:"path"`
	FormatTransformers []string `json:"format-transformers,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Binding is the Schema for the bindings API
// +k8s:openapi-gen=true

// GetValueFrom is the type that would appear in a CRD to allow dynamic configuration
type GetValueFrom struct {
	GetValueFrom composableGetValueFrom `json:"getValueFrom"`
}