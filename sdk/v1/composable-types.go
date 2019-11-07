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
