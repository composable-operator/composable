# Composable Operator

Composable is an overlay operator that can wrap any resource (native Kubernetes or CRD instance) and allows it to be dynamically configurable. Any field of the underlying resource can be specified with a reference to a secret or a configmap.

The Composable Operator enables the complete declarative executable specification of collections of inter-dependent resources.

## Installation

To install the Composable operator, do the following:
```shell
git clone git@github.ibm.com:seed/composable.git
./composable/hack/install-composable.sh [namespace]
```
An optional [namespace] argument specifies the namespace in which the controller pod will run. If a namespace is not provided, the controller pod will run in the `default` namespace.

## Examples

```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Composable
metadata:
  name: comp
spec:
  template: 
    apiVersion: ibmcloud.ibm.com/v1alpha1
    kind: Service
    metadata:
      name: mymessagehub
    spec:
      instancename: mymessagehub
      service: Event Streams
      plan: 
        getValueFrom:
          kind: Secret 
          name: mysecret
          path: data.plan
```

In this example, the field `plan` of the `Service.ibmcloud` instance is specified by referring to a secret. When the composable operator is created, its controller tries to read the secret and obtains the data needed for this field. If the secret is available, it then creates the `Service.ibmcloud` resource with the proper configuration. If the secret does not exist, the Composable controller keeps re-trying until it becomes available.

Here is another example:
```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Composable
metadata:
  name: comp
spec:
  template: 
    apiVersion: ibmcloud.ibm.com/v1alpha1
    kind: Service
    metadata:
      name:
        getValueFrom:
          kind: ConfigMap
          name: myconfigmap
          path: data.name
    spec:
      instancename: 
        getValueFrom:
          kind: ConfigMap
          name: myconfigmap
          path: data.name
      service: Event Streams
      plan: 
        getValueFrom:
          kind: Secret 
          name: mysecret
          path: data.plan
 ```
 
 In the above example, the name of the underlying `Service.ibmcloud` instance is obtained from a `configmap` and the same 
 name is used for the field `instancename`. This allows flexibility in defining configurations, and promotes the reuse 
 of yamls by alleviating hard-wired information.
 Moreover, it can be used to configure with data that is computed dynamically as a result of the deployment of some other 
 resource.
 The `getValueFrom` element can point to any K8s and its extensions object. The kind of teh object is defined by the`kind` 
 element; the object name is defined by teh `name` elements, and finally, the path to the data is defined by the value of
 the `path` element, which is a string with dots as a delimiter. 
 
 
## Namespaces

The `getValueFrom` section can have a field for the `namespace`. In this case, the specified namespace is used to look up the object that is being referenced. If the `namespace` field is absent then the namespace of the Composable object itself is used instead.

The template object can have a `namespace` specified in its `metadata` section. In that case, the underlying object is created in that namespace. If the template does not have a `namespace` field, then the object is created in the namespace of the `Composable` itself.

## Deletion

When the Composable object is deleted, the underlying object is deleted as well.
However, currently if the user deletes the underlying object manually, it is not automatically recreated (future work).
           
