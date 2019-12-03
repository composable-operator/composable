module github.com/ibm/composable/sdk

go 1.12

require (
	github.com/onsi/ginkgo v1.6.0
	github.com/onsi/gomega v1.4.2
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45 // indirect
	google.golang.org/appengine v1.5.0 // indirect
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	sigs.k8s.io/controller-runtime v0.2.0
)

replace (
	k8s.io/api => k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go => k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.2.0
)
