## Installing Prerequisites

Composable operator v0.1.2 has an integrated admission control webhook to perform validation of the reference specifications in Composable. The webhook must be served via HTTPS with proper certificates. As a prerequisite [cert-manager](https://docs.cert-manager.io/en/latest/getting-started/install/kubernetes.html#) must be installed in your cluster to manage certificate creation and injection.  

To install cert-manager, run the following script:

```bash
curl -sL https://raw.githubusercontent.com/IBM/composable/master/hack/install-cert-manager.sh | bash
```
