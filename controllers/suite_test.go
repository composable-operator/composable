/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	webappv1 "github.com/ibm/composable/api/v1alpha1"
	"github.com/ibm/composable/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{envtest.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {

	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))
	// SetDefaultEventuallyPollingInterval(1 * time.Second) // by default poll every 10 milliseconds
	SetDefaultEventuallyTimeout(10 * time.Second) // by default the polling is up to 1 second

	By("bootstrapping test environment")
	t := true
	if os.Getenv("TEST_USE_EXISTING_CLUSTER") == "true" {
		testEnv = &envtest.Environment{
			UseExistingCluster: &t,
		}
	} else {
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases"),
				filepath.Join("./testdata", "crds")}}
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = webappv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	syncPeriod := 30 * time.Second // set a sync period
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{SyncPeriod: &syncPeriod, MetricsBindAddress: "0"})
	Expect(err).NotTo(HaveOccurred())

	err = (&ComposableReconciler{
		Client:          k8sManager.GetClient(),
		Log:             ctrl.Log.WithName("controllers").WithName("SecretScope"),
		DiscoveryClient: discovery.NewDiscoveryClientForConfigOrDie(cfg),
		Scheme:          k8sManager.GetScheme(),
		Config:          k8sManager.GetConfig(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	client := k8sManager.GetClient()

	stop = test.StartTestManager(k8sManager)
	testNs := test.SetupKubeOrDie(cfg, "test-ns-")
	testContext = test.NewTestContext(client, testNs)

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	close(stop)
	time.Sleep(1 * time.Second)
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
