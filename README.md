# Composable Operator

Composable is an overlay operator that can wrap any resource (native Kubernetes or CRD instance) and allows it to be dynamically configurable. Any field of the underlying resource can be specified with a reference to a secret or a configmap.

The Composable Operator enables the complete declarative executable specification of collections of inter-dependent resources.

Example:

```
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Composable
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
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
          secretKeyRef: 
            name: mysecret
            key: plan
```

In this example, the field `plan` of the `Service.ibmcloud` instance is specified by referring to a secret. When the composable operator is created, its controller tries to read the secret and obtains the data needed for this field. If the secret is available, it then creates the `Service.ibmcloud` resource with the proper configuration. If the secret does not exist, the Composable controller keeps re-trying until it becomes available.
