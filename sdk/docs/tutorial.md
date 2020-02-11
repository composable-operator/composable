# Composable SDK Tutorial

Kubernetes object specifications often require constant values for their fields. When deploying an entire application
with many different resources, this limitation often results in the need for staged deployments, because some resources
have to be deployed first in order to determine what data to provide for the specifications of dependent resources.
This undermines the declarative nature of Kubernetes object specification and requires workflows, manual step-by-step
instructions and/or brittle automated scripts for the deployment of applications as a whole.

The Composable SDK can be used to add cross-resource references to any existing CRD, so that values no longer
need to be hardwired. This feature allows dynamic configuration of a resource, meaning that its fields can be 
resolved after it has been deployed. 

In this tutorial, we add cross-references to the `memcached-operator`, which is provided as a sample for `operator-sdk`.

## Modifying Memcached Types

To start, we need to modify the schema for `Memcached` objects so that they allows references, instead of hard wired values.

The original `Memcached` spec is as follows:

```golang
type MemcachedSpec struct {
	// Size is the size of the memcached deployment
	Size int32 `json:"size"`
}
```

We modify this `struct` as shown below:
```golang
import (
	sdk "github.com/ibm/composable/sdk"
	...
)

// MemcachedSpec defines the desired state of Memcached
// +k8s:openapi-gen=true
type MemcachedSpec struct {
	// Size is the size of the memcached deployment
	Size sdk.ObjectRef `json:"size"`
}
```

The `sdk.ObjectRef` types is the schema for making a reference to a Kubernetes object. 
Here is its definition:
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

For a detailed explanation of how to specify an object reference according to this schema, see [here](https://github.com/IBM/composable/blob/master/README.md#getvaluefrom-elements).

Given the new specification for `Memcached`, a sample CR can be written as:
```yaml
apiVersion: cache.example.com/v1alpha1
kind: Memcached
metadata:
  name: example-memcached
spec:
  # Add fields here
  # size: 3
  size: 
    getValueFrom:
      kind: ConfigMap
      name: myconfigmap
      namespace: memcached
      path: '{.data.size}'
      format-transformers:
      - "StringToInt32"
```

This says that the value for the `size` field is to be obtained from a `ConfigMap` named `myconfigmap` in the
`memcached` namespace. The `path` field indicates how to obtain the desired data in the `ConfigMap` and uses
`jsonpath`. The `format-transformers` field indicates that the data needs to be transformed from `string` to `int32`.

## Adding Resolved Types

In addition to modifying the original `Memcached` specification type, we also need to add types for the resolved objects.
These will help the Composable sdk to cast resolved object to an appropriate and convenient type for use in the `Memcached` controller.

```golang
// MemcachedSpecResolved contains the resolved schema for Memcached spec
type MemcachedSpecResolved struct {
	// Size is the size of the memcached deployment
	Size int32 `json:"size"`
}

// MemcachedResolved contains the resolved schema for Memcached objects
type MemcachedResolved struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemcachedSpecResolved `json:"spec,omitempty"`
	Status MemcachedStatus       `json:"status,omitempty"`
}
```


## Modifying the Memcached Controller

We now modify the `Memcached` reconciler to consume the new specifcation and call on the Composable SDK to resolve requests.

First, we need to modify the `ReconcileMemcached` type to include a Kubernetes configuration, which the Composable SDK needs.

```golang
// ReconcileMemcached reconciles a Memcached object
type ReconcileMemcached struct {
	client   client.Client
	scheme   *runtime.Scheme
	resolver sdk.ResolveObject
}
```

The `ReconcileMemcached` struct now has a field `resolver`, which implements the interface `ResolveObject` that offers a method for resolving
cross-resource references.
We modify the creation of the reconciler accordingly:

```golang
// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cfg := mgr.GetConfig()
	return &ReconcileMemcached{client: mgr.GetClient(), scheme: mgr.GetScheme(),
		resolver: sdk.KubernetesResourceResolver{
			Client:          mgr.GetClient(),
			ResourcesClient: discovery.NewDiscoveryClientForConfigOrDie(cfg),
		},
	}
}
```

Finally, we add code in the reconciler to resolve objects using the Composable SDK:

```golang
func (r *ReconcileMemcached) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Memcached.")

	// Fetch the Memcached instance
	memcached := &cachev1alpha1.Memcached{}
	err := r.client.Get(context.TODO(), request.NamespacedName, memcached)
	if err != nil {
		... // same as before
	}

	// Resolve the memcached instance
	resolved := &cachev1alpha1.MemcachedResolved{}
	err = r.resolver.ResolveObject(context.TODO(), memcached, resolved)
	// Fix this to have more info on the nature of the error
	if err != nil {
		if sdk.IsRefNotFound(err) {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Check if the Deployment already exists, if not create a new one
	deployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: memcached.Name, Namespace: memcached.Namespace}, deployment)
	if err != nil && errors.IsNotFound(err) {
		// Define a new Deployment
		dep := r.deploymentForMemcached(resolved)
		reqLogger.Info("Creating a new Deployment.", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
	

```

The `ResolveObject` function takes a context, an object to resolve (in this example `memcached`), and
an empty object with the right type to return the resolved object (in this example `resolved`).
In this example, `resolved` is of type `MemcachedResolved` as specified above. 

The function `sdk.ResolveObject` returns an error and the Composable SDK offers a variety of functions to determine the nature of the error.
This helps to determine whether the calling reconciler should return this error or not. This is used to distinguish errors that are minor and could be fixed by immediately retrying from errors that may indicate a stronger failure, such as an ill-formed yaml (in which case there is no need to retry right away).
In this case, if the reference is not found, we return the error to retry. Otherwise, we do not return the error.

Notice that the signature of the `deploymentForMemcached` function has changed and that it takes a `MemcachedResolved` object now.
Also when the size needed in the code (not shown here), it must be obtained from `resolved` instead of `memcached`.


## Putting It All Together

To test our enhanced operator, we must first run:

```sh
make code-gen
```

to re-run the code generation. Then the following will rebuild and push the image, and reinstall the operator.

```sh
operator-sdk build $IMAGE
docker push $IMAGE
make install
```

This will create the `example-memcached` CR as modified (shown above), 
but there are initially no deployments corresponding to this memcached.
We can create the configmap that is referenced in the CR:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: myconfigmap
  namespace: memcached
data:
  size: "3"
```

When we create this configmap, the `memcached` reconcile is able to successfully resolve the object using the Composable SDK and 
obtain the required number of replicas. Checking again to see deployments and pods shows that they have
been created successfully!

See the complete code for [`memcached-types.go`](./memcached-types.md) and [`memcached-controller.go`](./memcached-controller.md).
