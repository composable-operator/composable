# Composable SDK

[![Build Status](https://travis-ci.com/IBM/composable.svg?branch=master)](https://travis-ci.com/IBM/composable)
[![Go Report Card](https://goreportcard.com/badge/github.com/IBM/composable)](https://goreportcard.com/report/github.com/IBM/composable)
[![GoDoc](https://godoc.org/github.com/IBM/composable/sdk?status.svg)](https://godoc.org/github.com/IBM/composable/sdk)

Kubernetes object specifications often require constant values for their fields. When deploying an entire application
with many different resources, this limitation often results in the need for staged deployments, because some resources
have to be deployed first in order to determine what data to provide for the specifications of dependent resources.
This undermines the declarative nature of Kubernetes object specification and requires workflows, manual step-by-step
instructions and/or brittle automated scripts for the deployment of applications as a whole.

The Composable SDK can be used to add cross-resource references to any existing CRD, so that values no longer
need to be hardwired. This feature allows dynamic configuration of a resource, meaning that its fields can be 
resolved after it has been deployed. 

See this [tutorial](./docs/tutorial.md), in which we add cross-references to the `memcached-operator` (provided as a sample for `operator-sdk`) using the Composable SDK.

## Installation

To install, run:
```
go get github.com/ibm/composable/sdk
```

## Types

The Composable SDK offers the following types.

```golang
type ObjectRef struct {
	GetValueFrom ComposableGetValueFrom `json:"getValueFrom"`
}

type ComposableGetValueFrom struct {
	Kind               string   `json:"kind"`
	APIVersion         string   `json:"apiVersion,omitempty"`
	Name               string   `json:"name,omitempty"`
	Labels             []string `json:"labels,omitempty"`
	Namespace          string   `json:"namespace,omitempty"`
	Path               string   `json:"path"`
	FormatTransformers []string `json:"format-transformers,omitempty"`
}
```

An `ObjectRef` can be used to specify the type of any field of a CRD definition, allowing the value to be determined dynamically.
For a detailed explanation of how to specify an object reference according to this schema, see [here](https://github.com/IBM/composable/blob/master/README.md#getvaluefrom-elements).

## Functions

The Composable SDK offers the following function for resolving the value of cross-resource references.

```golang
func ResolveObject(r client.Client, config *rest.Config, object interface{}, resolved interface{}, composableNamespace string) *ComposableError 
```

The function `ResolveObject` takes a Kubernetes client and configuration, and `object` to resolve, and a blank object
`resolved` that will contain the result, as well as a namespace. The namespace is the default used when references
do not specify one. This function will cast the result to the type of the `resolved` object, provided that
appropriate data transforms have been included in the reference definitions (see [tutorial](./docs/tutorial.md) for an example).

The `ResolveObject` function uses caching for looking up object kinds, as well as for the objects themselves, in order
to ensure that a consistent view of the data is obtained. If any data is not available at the time of the lookup,
it returns an error. So this function either resolves the entire object or it doesn't -- there are no partial results.

The return value is a `ComposableError`:

```golang
type ComposableError struct {
	Error error
	// This indicates that the consuming Reconcile function should return this error
	ShouldBeReturned bool
}
```

The type `ComposableError` contains the error, if any, and a boolean `ShouldBeReturned` to indicate whether the calling reconciler
should return this error or not. This is used to distinguish errors that are minor and could be fixed by immediately retrying
from errors that may indicate a stronger failure, such as an ill-formed yaml (in which case there is no need to retry right away).
