<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Composable Operator](#composable-operator)
  - [Installation Composable](#installation-composable)
  - [Removing Composable](#removing-composable)
  - [Examples](#examples)
    - [An example when a Kubernetes `ConfigMap` created based on a Kubernetes Service](#an-example-when-a-kubernetes-configmap-created-based-on-a-kubernetes-service)
    - [An example of Service.ibmcloud.ibm.com](#an-example-of-serviceibmcloudibmcom)
  - [getValueFrom elements](#getvaluefrom-elements)
  - [The input object group and version discovery algorithm](#the-input-object-group-and-version-discovery-algorithm)
  - [Format transformers](#format-transformers)
  - [Namespaces](#namespaces)
  - [Deletion](#deletion)
  - [Field path discovery](#field-path-discovery)
    - [Limitations](#limitations)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->


*Note:* The project uses Golang modules, in order to  activate module support, please set the the `GO111MODULE` 
environment variable to `on`. [How to Install and Activate Module Support](https://github.com/golang/go/wiki/Modules#how-to-install-and-activate-module-support)
 
# Composable Operator

Composable is an overlay operator that can wrap any resource (native Kubernetes or CRD instance) and allows it to be 
dynamically configurable. Any field of the underlying resource can be specified with a reference to any field of other 
Kubernetes objects.

A cloud application is composed of many heteregeneous resources whose deployment must often be staged. For example,
a resource that depends on Kafka might have a field in its Spec expecting a Kafka Admin URL. In order to obtain a Kafka admin URL,
Kafka itself must be first successfully deployed with its own operator. So the yamls that deploy Kafka and the resource cannot all be deployed at once. This is why deployment of whole applications requires a playbook, step-by-step instructions, or scripts that may be brittle and error-prone.

The Composable operator allows all yamls of an application to be deployed at once, in one step, by supporting inter-yaml references.
In the example above, the resource is wrapped in a Composable object as a template and the Kafka Admin URL field has a reference
to another object that will eventually hold the required data. When this data becomes available the Composable controller deploys its underlying
resource. The composable operator therefore uses native Kubernetes mechanisms to dynamically stage deployments of a collection of resources.


## Installation Composable

To install the latest release of Composable, run the following script:

```bash
curl -sL https://raw.githubusercontent.com/IBM/composable/master/hack/install-composable.sh | bash 
```
Composable will be installed in the `composable-operator` namespace

## Removing Composable

To remove Composable, run the following script:

```bash
curl -sL https://raw.githubusercontent.com/IBM/composable/master/hack/uninstall-composable.sh | bash 
```
## Examples

Here we provide several examples of Composable usage, of course its possible usage is not restricted by the provided use 
cases. Others can be added later. 

Folder with all examples can be found in [samples](./config/samples)
   
### An example when a Kubernetes `ConfigMap` created based on a Kubernetes Service
Let's assume that we have a Kubernetes `Service`, which is part of another deployment, but we would like to create an automatic 
binding of our deployment objects with this `Service`. With help of `Composable` we can automatically create a `ConfigMap` 
with a `Service` parameter(s), e.g. the port number, whose name is `http`     

The `Service` yaml [file](./config/samples/myService.yaml) might looks like:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: myservice
  namespace: default
spec:
  sessionAffinity: None
  type: ClusterIP
  selector:
    app: MyApp
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: 9376
```

The following [file](./config/samples/compCM.yaml) contains the `Composable` definition:

```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Composable
metadata:
  name: to-cm
spec:
  template:
    apiVersion: "v1"
    kind: ConfigMap
    metadata:
      name: myconfigmap
    data:
      servicePort:
        getValueFrom:
          kind: Service
          name: myservice
          namespace: default
          path: '{.spec.ports[?(@.name=="http")].port}}'
          format-transformers:
            - ToString
``` 
You can see the detail explanation of the `getValueForm` fields below, but the purpose of the object is to create a  
`ConfigMap` named `myconfigmap` and set `servicePort` to be equal to the port named `http` in the `Service` object named
`myservice` in the `default` namespace.  
A Composable  and a created object (`myconfigmap`) will be in teh same namespace, but input objects can be in any namespaces.

### An example of Service.ibmcloud.ibm.com

Composable-operator project works tightly with 2 other related projects: [SolSA - Solution Service Architecture](https://github.com/IBM/solsa)
and [cloud-operators](https://github.com/IBM/cloud-operators). The [samples](./config/samples) directory has 3 different 
examples of creation/configuration of `Service.ibmcloud.ibm.com` from the `cloud-opertors` project.

Here is one  of them 

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
            path: '{.data.plan}'
            format-transformers:
            - "Base64ToString" 
        
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
          namespace: default
          path: {.data.name}
    spec:
      instancename: 
        getValueFrom:
          kind: ConfigMap
          name: myconfigmap
          namespace: default
          path: {.data.name}
      service: Event Streams
      plan: 
        getValueFrom:
          kind: Secret 
          name: mysecret
          namespace: default
          path: {.data.planKey}
 ```
 In this example, the name of the underlying `Service.ibmcloud` instance is obtained from a `configmap` and the same 
 name is used for the field `instancename`. This allows flexibility in defining configurations, and promotes the reuse 
 of yamls by alleviating hard-wired information.
 Moreover, it can be used to configure with data that is computed dynamically as a result of the deployment of some other 
 resource.
 
 The `getValueFrom` element can point to any K8s and its extensions object. The kind of the object is defined by the`kind` 
 element; the object name is defined by the `name` elements, and finally, the path to the data is defined by the value of
 the `path` element, which is a string with dots as a delimiter. 
 
## getValueFrom elements

The `getValueFrom` element should be a single child of the parent element and can contain the following sub-fileds:

Field | Is required | Format/Type | Comments
----- | ------------|-------------|-----------------
 kind | Yes | String | Kind of the input object
 apiVersion | No | String | Defines a K8s Api group and version of the checking object. Helps to resolve conflicts, when the same `Kind` defined in several API groups and there are several supported API versions
 name | Yes/No | String | Name of the input object. Either name or labels should be defined
 labels | Yes/No | [string]string | Labels of input objects. Either name or labels should be defined
 namespace | No | String | Namespace of the input object, if isn't defined, the ns of the `Composable` operator will be checked
 path | Yes | String | The `jsonpath` formatted path to the checked filed
 format-transformers | No | Array of predefined strings | Used for value type transformation, see [Format transformers](#format-transformers)

Notes:
* Ether `name` or `labels` of the input object should be defined. If neither of both the fields are defined, an error will be generated.
* The labels based search should return a single input object. 

## The input object group and version discovery algorithm

* If `apiVersion` of the input object is specified, Composable controller will try to discover an input object with the given group and version. 
	* If the given Kind is not part of the provided group, an error will be generated, despite of the Kind existence in other groups.
	* If the provided version is not supported, an error will be generated, despite of other versions existence.
	* Kubernetes core objects don't have group, so only version should be specified, e.g. `v1`.
* If `apiVersion` is not provided and the given Kind is part of the Kubernetes core group, the core group will be used, despite of the Kind existence in other groups.
* If `apiVersion` is not provided and the given Kind exists in several groups, and doesn't exist in the Kuberntes Core group, an error will be generated. 

## Format transformers

Sometimes, types of an input value and expected output value are not compatable, in order to resolve this issue, 
`Composable` supports several predefined transformers. They can be defined as a string array, so output of the previous 
transformer's will be input to next one.
When you define a `Composable` object, it is your responsibility to put in a correct order the transformers.

Currently `Composable` supports the following transformers:

Transformer | Transformation 
------------| ---------------
`ToString` | returns a native string representation of any object
`ArrayToCSString` | returns a comma-separated string from array's values 
`Base64ToString` | decodes a `base64` encoded string
`StringToBase64` | encodes a string to `base64` 
`StringToInt` | transforms a string to an integer
`StringToFloat` | transforms a string to a float
`StringToBool` | transforms a string to boolean
`JsonToObject` | transforms a JSON string to an object
`ObjectToJson` | transforms an object to a JSON string

The data transformation roles are:
* If there is no data transformers - original data format will be used, include complex structures such as maps or arrays.
* Transformers from the format-transformers array executed one after another according to their order. Which allows 
creation of data transformation pipelines. For example, the following snippet defines transformation from a base64 
encoded string to a plain string and after that to integer. This transformation can be useful to retrieve data from Secrets.
 
```yaml
format-transformers:
 - Base64ToString
 - StringToInt
```  
 
## Namespaces

The `getValueFrom` definition includes the destination `namespace`, the specified namespace is used 
to look up the referenced object. Otherwise, the `namespace` of the `Composable` object is checked.

The template object should be created in the same n`amespaces` as the `Composable` object. Therefore, we recommend do not
define `namespace` in the template. If the namespace field is defined and its value does not equal to the `Composable`
object namespace, no objects will be created, and `Composable` object status will contain an error.  

## Deletion

When the Composable object is deleted, the underlying object is deleted as well.
If the user deletes the underlying object manually, it is automatically recreated.


## Field path discovery

We use a `jsonpath` parser from `go-client` to define path to the resolving files. Here some examples:

* `{.data.key-name}` - returns a path to the key named `key-name` from a `ConfigMap` or from a `Secret`
* `{.spec.ports[?(@.name==“http”)].port}}` - takes port value from a port named `http` from the `ports` array`

If one of the element names, e.g. a key in a `ConfigMap` or `Secret`, contains dots, the dots should be escaped. 
For example, if we have the following Secret definition:
```yaml
apiVersion: v1
data:
  tls.crt: base64data
  tls.key: base64data
kind: Secret
metadata:
  name: data-center
  namespace: default
type: Opaque
``` 
In order to access the `tls.key` data, the jsonpath should be `'{.data.tls\.key}'`

### Limitations

Due to 
[issue #72220](https://github.com/kubernetes/kubernetes/issues/72220), `jsonpath` doesn't support regular expressions 
in json-path


           
