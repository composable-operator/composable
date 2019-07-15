<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Composable Operator](#composable-operator)
  - [Installation](#installation)
  - [Examples](#examples)
  - [Namespaces](#namespaces)
  - [Field path discovery](#field-path-discovery)
    - [Limitations](#limitations)
  - [Data formatting roles](#data-formatting-roles)
  - [Deletion](#deletion)
  - [TODO](#todo)
  - [Questions](#questions)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

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
          # any Kubernetes or CRD Kind
          kind: Secret 
          
          # The discovered object name
          name: mysecret
          
          # The jsonpath style path to the field
          # Example: get value of nodePort from a service ports array, when the port name is "http"
          path: {.spec.ports[?(@.name==“http”)].port}}
          
          # [Optional] the discovered object's namespace, if doesn't present, the Composable object namespace will be used
          # namespace: my-namespace
          
          # [Optional] format-transformers, array of the available values, which are:
          # ToString 		- transforms interface to string
          # String2Int 		- transforms string to integer
          # Base642String  	- decodes a base64 encoded string to a plain one
          # String2Base64	- encodes a plain string to a base 64 encoded string
          # String2Float    - transforms string to float
          # Array2CSString  - transforms arrays of objects to a comma-separated string
          # if presents, the operator will transform discovered data to the wished format
          # Example: transform data from base64 encoded string to an integer
          # format-transformer:
          #  - base642String
          #  - string2Int
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
          path: {.data.name}
    spec:
      instancename: 
        getValueFrom:
          kind: ConfigMap
          name: myconfigmap
          path: {.data.name}
      service: Event Streams
      plan: 
        getValueFrom:
          kind: Secret 
          name: mysecret
          path: {.data.plan}
 ```
 
 In the above example, the name of the underlying `Service.ibmcloud` instance is obtained from a `configmap` and the same 
 name is used for the field `instancename`. This allows flexibility in defining configurations, and promotes the reuse 
 of yamls by alleviating hard-wired information.
 Moreover, it can be used to configure with data that is computed dynamically as a result of the deployment of some other 
 resource.
 The `getValueFrom` element can point to any K8s and its extensions object. The kind of the object is defined by the`kind` 
 element; the object name is defined by the `name` elements, and finally, the path to the data is defined by the value of
 the `path` element, which is a string with dots as a delimiter. 
 
 
## Namespaces

The `getValueFrom` section can have a field for the `namespace`. In this case, the specified namespace is used 
to look up the object that is being referenced. If the `namespace` field is absent then the namespace of 
the Composable object itself is used instead.

The template object can have a `namespace` specified in its `metadata` section. In that case, the underlying object is 
created in that namespace. If the template does not have a `namespace` field, then the object is created in 
the namespace of the `Composable` itself.

## Field path discovery

We use a `jsonpath` parser from `go-client` to define path to the resolving files. Here some examples:

* `{.data.key-name}` - returns a path to the key named `key-name` from a `ConfigMap` or from a `Secret`
* `{.spec.ports[?(@.name==“http”)].port}}` - takes port value from a port named `http` from the `ports` array

### Limitations

Due to 
[issue #72220](https://github.com/kubernetes/kubernetes/issues/72220), `jsonpath` doesn't support regular expressions 
in json-path

## Data formatting roles

Frequently data retrieved from another object needs to be transformed to another format. `format-transformers` help to 
do it. Here are the data transformation roles:
 
* If there is no data transformers  -  original data format will be used, include complex structures such as maps or arrays.
* Transformers from the data-transformers array executed one after another according to their order. Which allows 
creation of data transformation pipelines. For example, teh following snippet defines a data transformation from a base64 
encoded string to a plain string and after that to integer. This transformation can be useful to retrieve data from Secretes.
 
```yaml
format-transformers:
 - Base642String
 - String2Int
```  

* `ToString` - returns a native string representation of any object
* `Array2CSString` - returns a comma-separated string from array's values 

## Deletion

When the Composable object is deleted, the underlying object is deleted as well.
However, currently if the user deletes the underlying object manually, it is not automatically recreated (future work).



## TODO

* Extend e2e test framework
* Allow transformers as a function, extendability
* Support waiting conditions - wait when a checking resource is Online, Ready, or Running, and after that do other 
operations: deploy, retrieve value and deploy, start a job ...
* Support resource monitoring, and automatically propagate the updates.
* Replace logger

## Questions

* Should we provide a separate discovery mechanism to retrieve values from Secrets and ConfigMap.
	* The path can be changed from `{.Object.data.key}` to `data.key` or even just `key`. 
	* In any case, we probably have to support transformers.`  
* Should we eliminate the prefix `Object` in the jsonpath? 

           
