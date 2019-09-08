/*
 * Copyright 2019 IBM Corporation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package composable

import (
	"flag"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/ibm/composable/pkg/apis"
	"github.com/ibm/composable/pkg/context"
	"github.com/ibm/composable/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	c        client.Client
	cfg      *rest.Config
	testNs   string
	scontext context.Context
	t        *envtest.Environment
	stop     chan struct{}
)

func TestComposable(t *testing.T) {
	klog.InitFlags(flag.CommandLine)
	klog.SetOutput(GinkgoWriter)

	RegisterFailHandler(Fail)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	SetDefaultEventuallyTimeout(60 * time.Second)

	RunSpecs(t, "Composable Suite")
}

var _ = BeforeSuite(func() {

	t = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crds"),
			filepath.Join("./testdata", "crds")},
		ControlPlaneStartTimeout: 2 * time.Minute,
	}
	apis.AddToScheme(scheme.Scheme)

	var err error
	if cfg, err = t.Start(); err != nil {
		log.Fatal(err)
	}

	mgr, err := manager.New(cfg, manager.Options{})
	Expect(err).NotTo(HaveOccurred())

	c = mgr.GetClient()

	recFn := newReconciler(mgr)
	Expect(add(mgr, recFn)).NotTo(HaveOccurred())
	stop = test.StartTestManager(mgr)
	testNs = test.SetupKubeOrDie(cfg, "test-ns-")
	scontext = context.New(c, reconcile.Request{NamespacedName: types.NamespacedName{Name: "", Namespace: testNs}})

})

var _ = AfterSuite(func() {
	close(stop)
	t.Stop()
})

var _ = Describe("test Composable operations", func() {
	dataDir := "testdata/"
	unstrObj := unstructured.Unstructured{}
	strArray := []interface{}{"kafka01-prod02.messagehub.services.us-south.bluemix.net:9093",
		"kafka02-prod02.messagehub.services.us-south.bluemix.net:9093",
		"kafka03-prod02.messagehub.services.us-south.bluemix.net:9093",
		"kafka04-prod02.messagehub.services.us-south.bluemix.net:9093",
		"kafka05-prod02.messagehub.services.us-south.bluemix.net:9093"}

	It("Composable should successfully copy input data to the output object fields", func() {
		By("Deploy input Object")
		obj := test.LoadObject(dataDir+"inputDataObject.yaml", &unstructured.Unstructured{})
		test.CreateObject(scontext, obj, true, 0)

		By("Deploy Composable object")
		comp := test.LoadCompasable(dataDir + "compCopy.yaml")
		test.PostInNs(scontext, &comp, true, 0)
		Eventually(test.GetObject(scontext, &comp)).ShouldNot(BeNil())

		By("Get Output object")
		groupVersionKind := schema.GroupVersionKind{Kind: "OutputValue", Version: "v1", Group: "test.ibmcloud.ibm.com"}
		unstrObj.SetGroupVersionKind(groupVersionKind)
		objNamespacedname := types.NamespacedName{Namespace: testNs, Name: "comp-out"}
		klog.V(5).Infof("Get Object %s\n", objNamespacedname)
		Eventually(test.GetUnstructuredObject(scontext, objNamespacedname, &unstrObj)).Should(Succeed())
		testSpec, ok := unstrObj.Object[spec].(map[string]interface{})
		Ω(ok).Should(BeTrue())

		By("copy intValue")
		// TODO should we check int type
		Ω(testSpec["intValue"]).Should(BeEquivalentTo(12))

		By("copy floatValue")
		Ω(testSpec["floatValue"]).Should(BeEquivalentTo(23.5))

		By("copy boolValue")
		Ω(testSpec["boolValue"]).Should(BeTrue())

		By("copy stringValue")
		Ω(testSpec["stringValue"]).Should(BeEquivalentTo("Hello world"))

		By("copy stringFromBase64")
		Ω(testSpec["stringFromBase64"]).Should(BeEquivalentTo("9376"))

		By("copy arrayStrings")
		Ω(testSpec["arrayStrings"]).Should(BeEquivalentTo(strArray))

		// TODO check why BeEquivalentTo doesn't work
		//By("copy arrayIntegers")
		//Ω(testSpec["arrayIntegers"]).Should(BeEquivalentTo([]interface{}{1,2,3,4}))

		//By("copy objectValue")
		//Ω(testSpec["objectValue"]).Should(BeEquivalentTo(map[string]interface {}{"family": "FamilyName", "first": "FirstName", "age": 27}))

		// TODO check why BeEquivalentTo doesn't work
		By("copy stringJson2Value")
		val, _ := Array2CSStringTransformer(strArray)
		Ω(testSpec["stringJson2Value"]).Should(BeEquivalentTo(val))

	})

})

var _ = Describe("IBM cloud-operators compatibility", func() {
	dataDir := "testdata/cloud-operators-data/"
	groupVersionKind := schema.GroupVersionKind{Kind: "Service", Version: "v1alpha1", Group: "ibmcloud.ibm.com"}

	Context("create Service instance from ibmcloud.ibm.com WITHOUT external dependencies", func() {
		It("should correctly create the Service instance", func() {

			comp := test.LoadCompasable(dataDir + "comp.yaml")
			test.PostInNs(scontext, &comp, true, 0)
			Eventually(test.GetObject(scontext, &comp)).ShouldNot(BeNil())

			objNamespacedname := types.NamespacedName{Namespace: scontext.Namespace(), Name: "mymessagehub"}
			unstrObj := unstructured.Unstructured{}
			unstrObj.SetGroupVersionKind(groupVersionKind)
			klog.V(5).Infof("Get Object %s\n", objNamespacedname)
			Eventually(test.GetUnstructuredObject(scontext, objNamespacedname, &unstrObj)).Should(Succeed())
			Eventually(test.GetState(scontext, &comp)).Should(Equal(OnlineStatus))

		})

		It("should delete the Composable and Service instances", func() {
			comp := test.LoadCompasable(dataDir + "comp.yaml")
			test.DeleteInNs(scontext, &comp, false)
			Eventually(test.GetObject(scontext, &comp)).Should(BeNil())
		})

	})

	Context("create Service instance from ibmcloud.ibm.com WITH external dependencies", func() {
		var objNamespacedname types.NamespacedName

		BeforeEach(func() {
			obj := test.LoadObject(dataDir+"mysecret.yaml", &v1.Secret{})
			test.PostInNs(scontext, obj, true, 0)
			objNamespacedname = types.NamespacedName{Namespace: scontext.Namespace(), Name: "mymessagehub"}
		})

		AfterEach(func() {
			obj := test.LoadObject(dataDir+"mysecret.yaml", &v1.Secret{})
			test.DeleteInNs(scontext, obj, false)
		})

		It("should correctly create the Service instance according to parameters from a Secret object", func() {
			comp := test.LoadCompasable(dataDir + "comp1.yaml")
			test.PostInNs(scontext, &comp, false, 0)
			Eventually(test.GetObject(scontext, &comp)).ShouldNot(BeNil())

			unstrObj := unstructured.Unstructured{}
			unstrObj.SetGroupVersionKind(groupVersionKind)
			klog.V(5).Infof("Get Object %s\n", objNamespacedname)
			Eventually(test.GetUnstructuredObject(scontext, objNamespacedname, &unstrObj)).Should(Succeed())
			Ω(getPlan(unstrObj.Object)).Should(Equal("standard"))
			Eventually(test.GetObject(scontext, &comp)).ShouldNot(BeNil())
			Eventually(test.GetState(scontext, &comp)).Should(Equal(OnlineStatus))
			test.DeleteInNs(scontext, &comp, false)
			Eventually(test.GetObject(scontext, &comp)).Should(BeNil())
		})

		It("should correctly create the Service instance according to parameters from a ConfigMap", func() {
			obj := test.LoadObject(dataDir+"myconfigmap.yaml", &v1.ConfigMap{})
			test.PostInNs(scontext, obj, true, 0)

			comp := test.LoadCompasable(dataDir + "comp2.yaml")
			test.PostInNs(scontext, &comp, false, 0)
			Eventually(test.GetObject(scontext, &comp)).ShouldNot(BeNil())

			unstrObj := unstructured.Unstructured{}
			unstrObj.SetGroupVersionKind(groupVersionKind)
			klog.V(5).Infof("Get Object %s\n", objNamespacedname)
			Eventually(test.GetUnstructuredObject(scontext, objNamespacedname, &unstrObj)).Should(Succeed())
			Ω(getPlan(unstrObj.Object)).Should(Equal("standard"))
			Eventually(test.GetObject(scontext, &comp)).ShouldNot(BeNil())
			Eventually(test.GetState(scontext, &comp)).Should(Equal(OnlineStatus))
			test.DeleteInNs(scontext, &comp, false)
			Eventually(test.GetObject(scontext, &comp)).Should(BeNil())
		})
	})

})

// returns service plan of Service.ibmcloud.ibm.com
func getPlan(objMap map[string]interface{}) string {
	if spec, ok := objMap[spec].(map[string]interface{}); ok {
		if plan, ok := spec["plan"].(string); ok {
			return plan
		}
	}
	return ""
}
