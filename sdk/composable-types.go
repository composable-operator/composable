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
