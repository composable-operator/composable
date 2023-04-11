# Composable SDK

:warning: This is not up-to-date anymore!

[![Build Status](https://travis-ci.com/IBM/composable.svg?branch=master)](https://travis-ci.com/IBM/composable)
[![Go Report Card](https://goreportcard.com/badge/github.com/composable-operator/composable)](https://goreportcard.com/report/github.com/composable-operator/composable)
[![GoDoc](https://godoc.org/github.com/composable-operator/composable/sdk?status.svg)](https://godoc.org/github.com/composable-operator/composable/sdk)

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
go get github.com/composable-operator/composable/sdk
```

## Types

The Composable SDK offers the following types to be used in a CRD definition:

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
For a detailed explanation of how to specify an object reference according to this schema, see [here](https://github.com/composable-operator/composable/blob/master/README.md#getvaluefrom-elements).

The Composable SDK offers the following types to be used as part of a Reconciler in a controller:

```golang
type ResolveObject interface {
	ResolveObject(ctx context.Context, in, out interface{}) error
}

type KubernetesResourceResolver struct {
	Client          client.Client
	ResourcesClient discovery.ServerResourcesInterface
}
```

The interface `ResolveObject` provides a function to resolve object references (see below). The struct `KubernetesResourceResolver`
implements it and can be used as part of a Reconciler struct in a CRD controller (see [tutorial](./docs/tutorial.md)). It requires a `Client` and a
`ServerResourceInterface` used to query Kubernetes about existing resources.

A `ServerResourceInterface` can be instantiated as follows:

```golang
discovery.NewDiscoveryClientForConfigOrDie(cfg)
```

where `discovery` is the package `k8s.io/client-go/discovery`, and `cfg` is a `rest.Config`.


## Functions

The Composable SDK offers the following function for resolving the value of cross-resource references.

```golang
func (k KubernetesResourceResolver) ResolveObject(ctx context.Context, object, resolved interface{}) error {
```

The function `ResolveObject` takes a context, an `object` to resolve, and a blank object
`resolved` that will contain the result of resolving cross-resource references. 
It assumes that the input object has a namespace, which is then used as the default namespace when references 
do not specify one. This function will cast the result to the type of the `resolved` object, provided that
appropriate data transforms have been included in the reference definitions (see [tutorial](./docs/tutorial.md) for an example).

The `ResolveObject` function uses caching for looking up objects in order
to ensure that a consistent view of the data is obtained. If any data is not available at the time of the lookup,
it returns an error. So this function either resolves the entire object or it doesn't -- there are no partial results.

The return value of `ResolveObject` is an `error` and the Composable SDK offers a series of functions to determine
the nature of the error. This is used to decide whether the error needs to be returned by the Reconcile function or not.

```golang
func IsIllFormedRef(err error) bool 

func IsKindNotFound(err error) bool 

func IsObjectNotFound(err error) bool 

func IsValueNotFound(err error) bool 

func IsRefNotFound(err error) bool 
```

Function `IsIllFormedRef` indicates that that a cross-resource reference is ill-formed (in which case retrying reconciliation
would probably not help). Function `IsKindNotFound` indicates that the kind of the reference does not exist.
`IsObjectNotFound` indicates that the object itself does not exist, and `IsValueNotFound` that the value within the object
does not exist. Finally, `IsRefNotFound` is true if either `IsKindNotFound`, `IsObjectNotFound`, or `IsValueNotFound` are true.
