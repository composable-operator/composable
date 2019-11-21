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

// +kubebuilder:object:generate=true
//ComposableGetValueFrom is the struct for Composable getValueFrom
type ComposableGetValueFrom struct {
	Kind               string   `json:"kind"`
	APIVersion         string   `json:"apiVersion,omitempty"`
	Name               string   `json:"name,omitempty"`
	Labels             []string `json:"labels,omitempty"`
	Namespace          string   `json:"namespace,omitempty"`
	Path               string   `json:"path"`
	FormatTransformers []string `json:"format-transformers,omitempty"`
}

// +kubebuilder:object:generate=true
// GetValueFrom is the type that would appear in a CRD to allow dynamic configuration
type GetValueFromType struct {
	GetValueFrom ComposableGetValueFrom `json:"getValueFrom"`
}
