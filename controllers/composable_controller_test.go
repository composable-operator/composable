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

package controllers

import (
	"github.com/ibm/composable/controllers/test"
	sdk "github.com/ibm/composable/sdk"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var testContext test.TestContext

var _ = Describe("test Composable operations", func() {
	dataDir := "testdata/"
	unstrObj := unstructured.Unstructured{}

	strArray := []interface{}{
		"kafka01-prod02.messagehub.services.us-south.bluemix.net:9093",
		"kafka02-prod02.messagehub.services.us-south.bluemix.net:9093",
		"kafka03-prod02.messagehub.services.us-south.bluemix.net:9093",
		"kafka04-prod02.messagehub.services.us-south.bluemix.net:9093",
		"kafka05-prod02.messagehub.services.us-south.bluemix.net:9093",
	}

	AfterEach(func() {
		// delete the Composable object
		comp := test.LoadComposable(dataDir + "compCopy.yaml")
		test.DeleteInNs(testContext, &comp, false)
		Eventually(test.GetObject(testContext, &comp)).Should(BeNil())

		obj := test.LoadObject(dataDir+"inputDataObject.yaml", &unstructured.Unstructured{})
		test.DeleteObject(testContext, obj, false)
		Eventually(test.GetObject(testContext, obj)).Should(BeNil())
	})

	It("Composable should successfully set default values to the output object", func() {
		By("Deploy Composable object")
		comp := test.LoadComposable(dataDir + "compCopy.yaml")
		test.PostInNs(testContext, &comp, false, 0)
		Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

		By("Get Output object")
		groupVersionKind := schema.GroupVersionKind{Kind: "OutputValue", Version: "v1", Group: "test.ibmcloud.ibm.com"}
		unstrObj.SetGroupVersionKind(groupVersionKind)
		objNamespacedname := types.NamespacedName{Namespace: testContext.Namespace(), Name: "comp-out"}
		Eventually(test.GetUnstructuredObject(testContext, objNamespacedname, &unstrObj)).Should(Succeed())
		testSpec, ok := unstrObj.Object[spec].(map[string]interface{})
		Expect(ok).Should(BeTrue())

		By("default intValue")
		Expect(testSpec["intValue"]).Should(BeEquivalentTo(10))

		By("default floatValue")
		Expect(testSpec["floatValue"]).Should(BeEquivalentTo(10.1))

		By("default boolValue")
		Expect(testSpec["boolValue"]).Should(BeFalse())

		By("default stringValue")
		Expect(testSpec["stringValue"]).Should(Equal("default"))

		By("default stringFromBase64")
		Expect(testSpec["stringFromBase64"]).Should(Equal("default"))

		By("default arrayStrings")
		Expect(testSpec["arrayStrings"]).Should(BeEquivalentTo([]interface{}{"aa", "bb", "cc"}))

		By("default arrayIntegers")
		Expect(testSpec["arrayIntegers"]).Should(BeEquivalentTo([]interface{}{int64(1), int64(0), int64(1)}))

		By("default objectValue")
		Expect(testSpec["objectValue"]).Should(BeEquivalentTo(map[string]interface{}{"family": "DefaultFamilyName", "first": "DefaultFirstName", "age": int64(-1)}))

		By("default stringJson2Value")
		Expect(testSpec["stringJson2Value"]).Should(BeEquivalentTo("default1,default2,default3"))
	})

	It("Composable should successfully copy values to the output object", func() {
		By("Deploy input Object")
		obj := test.LoadObject(dataDir+"inputDataObject.yaml", &unstructured.Unstructured{})
		test.CreateObject(testContext, obj, false, 0)
		Eventually(test.GetObject(testContext, obj)).ShouldNot(BeNil())

		groupVersionKind := schema.GroupVersionKind{Kind: "InputValue", Version: "v1", Group: "test.ibmcloud.ibm.com"}
		unstrObj.SetGroupVersionKind(groupVersionKind)
		objNamespacedname := types.NamespacedName{Namespace: "default", Name: "inputdata"}
		Eventually(test.GetUnstructuredObject(testContext, objNamespacedname, &unstrObj)).Should(Succeed())

		By("Deploy Composable object")
		comp := test.LoadComposable(dataDir + "compCopy.yaml")
		test.PostInNs(testContext, &comp, false, 0)
		Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())
		Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(OnlineStatus))

		By("Get Output object")
		groupVersionKind = schema.GroupVersionKind{Kind: "OutputValue", Version: "v1", Group: "test.ibmcloud.ibm.com"}
		unstrObj.SetGroupVersionKind(groupVersionKind)
		objNamespacedname = types.NamespacedName{Namespace: testContext.Namespace(), Name: "comp-out"}
		Eventually(test.GetUnstructuredObject(testContext, objNamespacedname, &unstrObj)).Should(Succeed())
		testSpec, ok := unstrObj.Object[spec].(map[string]interface{})
		Expect(ok).Should(BeTrue())

		By("copy intValue")
		// We use Eventually so the controller will be able to update teh destination object
		Expect(testSpec["intValue"]).Should(BeEquivalentTo(12))
		//
		By("copy floatValue")
		Expect(testSpec["floatValue"].(float64)).Should(BeEquivalentTo(23.5))

		By("copy boolValue")
		Expect(testSpec["boolValue"]).Should(BeTrue())

		By("copy stringValue")
		Expect(testSpec["stringValue"]).Should(Equal("Hello world"))

		By("copy stringFromBase64")
		Expect(testSpec["stringFromBase64"]).Should(Equal("9376"))

		By("copy arrayStrings")
		Expect(testSpec["arrayStrings"]).Should(Equal(strArray))

		By("copy arrayIntegers")
		Expect(testSpec["arrayIntegers"]).Should(Equal([]interface{}{int64(1), int64(2), int64(3), int64(4)}))

		By("copy objectValue")
		Expect(testSpec["objectValue"]).Should(Equal(map[string]interface{}{"family": "FamilyName", "first": "FirstName", "age": int64(27)}))

		By("copy stringJson2Value")
		val, _ := sdk.Array2CSStringTransformer(strArray)
		Expect(testSpec["stringJson2Value"]).Should(BeEquivalentTo(val))
	})
	It("Composable should successfully update values of the output object", func() {
		gvkIn := schema.GroupVersionKind{Kind: "InputValue", Version: "v1", Group: "test.ibmcloud.ibm.com"}
		gvkOut := schema.GroupVersionKind{Kind: "OutputValue", Version: "v1", Group: "test.ibmcloud.ibm.com"}
		objNamespacednameIn := types.NamespacedName{Namespace: "default", Name: "inputdata"}
		objNamespacednameOut := types.NamespacedName{Namespace: testContext.Namespace(), Name: "comp-out"}

		// unstrObj.SetGroupVersionKind(gvkOut)
		// First, the output object is created with default values, after that we deploy the inputObject and will check
		// that all Output object filed are updated.
		By("check that input object doesn't exist") // the object should not exist
		unstrObj.SetGroupVersionKind(gvkIn)
		Expect(test.GetUnstructuredObject(testContext, objNamespacednameIn, &unstrObj)()).Should(HaveOccurred())

		By("check that output object doesn't exist. If it does => remove it ") // the object should not exist, or we delete it
		unstrObj.SetGroupVersionKind(gvkOut)
		err2 := test.GetUnstructuredObject(testContext, objNamespacednameOut, &unstrObj)()
		if err2 == nil {
			test.DeleteObject(testContext, &unstrObj, false)
			Eventually(test.GetObject(testContext, &unstrObj)).Should(BeNil())
		}

		By("deploy Composable object")
		comp := test.LoadComposable(dataDir + "compCopy.yaml")
		test.PostInNs(testContext, &comp, false, 0)
		Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

		By("get Output object")
		unstrObj.SetGroupVersionKind(gvkOut)
		Eventually(test.GetUnstructuredObject(testContext, objNamespacednameOut, &unstrObj)).Should(Succeed())
		testSpec, ok := unstrObj.Object[spec].(map[string]interface{})
		Expect(ok).Should(BeTrue())

		// Check some of default the values
		By("check default intValue")
		Expect(testSpec["intValue"]).Should(BeEquivalentTo(10))

		By("check default stringFromBase64")
		Expect(testSpec["stringFromBase64"]).Should(Equal("default"))

		By("deploy input Object")
		obj := test.LoadObject(dataDir+"inputDataObject.yaml", &unstructured.Unstructured{})
		test.CreateObject(testContext, obj, false, 0)
		Eventually(test.GetObject(testContext, obj)).ShouldNot(BeNil())

		By("check updated inValue")
		unstrObj = unstructured.Unstructured{}
		unstrObj.SetGroupVersionKind(gvkOut)
		Eventually(func() (int64, error) {
			err := test.GetUnstructuredObject(testContext, objNamespacednameOut, &unstrObj)()
			if err != nil {
				return int64(0), err
			}
			testSpec, _ = unstrObj.Object[spec].(map[string]interface{})
			return testSpec["intValue"].(int64), nil
		}).Should(Equal(int64(12)))

		// Check other values
		By("check updated floatValue")
		Expect(testSpec["floatValue"].(float64)).Should(BeEquivalentTo(23.5))

		By("check updated boolValue")
		Expect(testSpec["boolValue"]).Should(BeTrue())

		By("check updated stringValue")
		Expect(testSpec["stringValue"]).Should(Equal("Hello world"))

		By("check updated stringFromBase64")
		Expect(testSpec["stringFromBase64"]).Should(Equal("9376"))

		By("check updated arrayStrings")
		Expect(testSpec["arrayStrings"]).Should(Equal(strArray))

		By("check updated arrayIntegers")
		Expect(testSpec["arrayIntegers"]).Should(Equal([]interface{}{int64(1), int64(2), int64(3), int64(4)}))

		By("check updated objectValue")
		Expect(testSpec["objectValue"]).Should(Equal(map[string]interface{}{"family": "FamilyName", "first": "FirstName", "age": int64(27)}))

		By("check updated stringJson2Value")
		val, _ := sdk.Array2CSStringTransformer(strArray)
		Expect(testSpec["stringJson2Value"]).Should(BeEquivalentTo(val))
	})
})

var _ = Describe("Validate input objects Api grop and version discovery", func() {
	Context("There are 3 groups that have Kind = `Service`. They are: Service/v1; Service.ibmcloud.ibm.com/v1alpha1 and Service.test.ibmcloud.ibm.com/v1", func() {
		dataDir := "testdata/"
		BeforeEach(func() {
			By("deploy K8s Service")
			kubeObj := test.LoadObject(dataDir+"serviceK8s.yaml", &v1.Service{})
			test.CreateObject(testContext, kubeObj, false, 0)
			Eventually(test.GetObject(testContext, kubeObj)).ShouldNot(BeNil())

			By("deploy test Service")
			tObj := test.LoadObject(dataDir+"serviceTest.yaml", &unstructured.Unstructured{})
			test.CreateObject(testContext, tObj, false, 0)
			Eventually(test.GetObject(testContext, tObj)).ShouldNot(BeNil())
		})

		AfterEach(func() {
			By("delete K8s Service")
			kubeObj := test.LoadObject(dataDir+"serviceK8s.yaml", &v1.Service{})
			test.DeleteObject(testContext, kubeObj, false)
			Eventually(test.GetObject(testContext, kubeObj)).Should(BeNil())

			By("delete test Service")
			tObj := test.LoadObject(dataDir+"serviceTest.yaml", &unstructured.Unstructured{})
			test.DeleteObject(testContext, tObj, false)
			Eventually(test.GetObject(testContext, tObj)).Should(BeNil())
		})

		It("Composable should correctly discover required objects, core service without apiVersion", func() {
			By("deploy Composable object " + "compServices.yaml")
			comp := test.LoadComposable(dataDir + "compServices.yaml")
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			By("get the output object and validate its fields")
			unstrObj := unstructured.Unstructured{}
			gvk := schema.GroupVersionKind{Kind: "OutputValue", Version: "v1", Group: "test.ibmcloud.ibm.com"}
			objNamespacedname := types.NamespacedName{Namespace: testContext.Namespace(), Name: "services-out"}

			unstrObj.SetGroupVersionKind(gvk)
			Eventually(test.GetUnstructuredObject(testContext, objNamespacedname, &unstrObj)).Should(Succeed())
			testSpec, ok := unstrObj.Object[spec].(map[string]interface{})
			Expect(ok).Should(BeTrue())

			Expect(testSpec["k8sValue"]).Should(Equal("None"))
			Expect(testSpec["testValue"]).Should(Equal("Test"))
		})

		It("Composable should correctly discover required objects, , core service with apiVersion=v1", func() {
			//By("deploy K8s Service")
			//kubeObj := test.LoadObject(dataDir+"serviceK8s.yaml", &v1.Service{})
			//test.CreateObject(testContext, kubeObj, false, 0)
			//Eventually(test.GetObject(testContext, kubeObj)).ShouldNot(BeNil())
			//
			//By("deploy test Service")
			//tObj := test.LoadObject(dataDir+"serviceTest.yaml", &unstructured.Unstructured{})
			//test.CreateObject(testContext, tObj, false, 0)
			//Eventually(test.GetObject(testContext, tObj)).ShouldNot(BeNil())

			By("deploy Composable object " + "compServicesV1.yaml")
			comp := test.LoadComposable(dataDir + "compServicesV1.yaml")
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			By("get the output object and validate its fields")
			unstrObj := unstructured.Unstructured{}
			gvk := schema.GroupVersionKind{Kind: "OutputValue", Version: "v1", Group: "test.ibmcloud.ibm.com"}
			objNamespacedname := types.NamespacedName{Namespace: testContext.Namespace(), Name: "services-out-v1"}

			unstrObj.SetGroupVersionKind(gvk)
			Eventually(test.GetUnstructuredObject(testContext, objNamespacedname, &unstrObj)).Should(Succeed())
			testSpec, ok := unstrObj.Object[spec].(map[string]interface{})
			Expect(ok).Should(BeTrue())

			Expect(testSpec["k8sValue"]).Should(Equal("None"))
			Expect(testSpec["testValue"]).Should(Equal("Test"))
		})

		It("Composable should fail to discover correct Service recourse, when there are several groups with the same Kind", func() {
			By("deploy Composable object " + "compAPIError.yaml")
			comp := test.LoadComposable(dataDir + "compAPIError.yaml")
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			By("Reload the Composable object")
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			By("validate that Composable object status is FailedStatus")
			Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(FailedStatus))
		})
		It("Composable should fail to discover correct Service recourse, when a wring API version is provided", func() {
			By("deploy Composable object " + "compAPIWrongVersionError.yaml")
			comp := test.LoadComposable(dataDir + "compAPIWrongVersionError.yaml")
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			By("Reload the Composable object")
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			By("validate that Composable object status is FailedStatus")
			Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(FailedStatus))
		})
	})
})

var _ = Describe("Find input objects according their labels", func() {
	Context("...", func() {
		dataDir := "testdata/"

		fGetValueFrom := func(unstrObj unstructured.Unstructured) map[string]interface{} {
			sp := unstrObj.Object[spec].(map[string]interface{})
			v := sp["testValue"].(map[string]interface{})
			return v[sdk.GetValueFrom].(map[string]interface{})
		}
		BeforeEach(func() {
			By("deploy test Service")
			tObj := test.LoadObject(dataDir+"serviceTest.yaml", &unstructured.Unstructured{})
			test.CreateObject(testContext, tObj, false, 0)
			Eventually(test.GetObject(testContext, tObj)).ShouldNot(BeNil())
		})

		AfterEach(func() {
			By("delete test Service")
			tObj := test.LoadObject(dataDir+"serviceTest.yaml", &unstructured.Unstructured{})
			test.DeleteObject(testContext, tObj, false)
			Eventually(test.GetObject(testContext, tObj)).Should(BeNil())
		})
		It("Deployment should fail with the `neither 'name' nor 'labels' are not defined` error", func() {
			By("deploy Composable object " + "compLabels.yaml" + " without name and labels")
			comp := test.LoadComposable(dataDir + "compLabels.yaml")
			unstrObj := unstructured.Unstructured{}
			unstrObj.UnmarshalJSON(comp.Spec.Template.Raw)
			valueFrom := fGetValueFrom(unstrObj)
			delete(valueFrom, sdk.Name)
			delete(valueFrom, sdk.Labels)
			comp.Spec.Template.Raw, _ = unstrObj.MarshalJSON()
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())
			Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(FailedStatus))
			Expect(test.GetStatusMessage(testContext, &comp)()).Should(ContainSubstring("missing required field \"name\" or \"labels\""))
			test.DeleteInNs(testContext, &comp, false)
			Eventually(test.GetObject(testContext, &comp)).Should(BeNil())
		})

		It("Deployment should fail with the `both 'name' and 'labels' cannot be defined` error", func() {
			By("deploy Composable object " + "compLabels.yaml" + " with both name and labels")
			comp := test.LoadComposable(dataDir + "compLabels.yaml")
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())
			Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(FailedStatus))
			Expect(test.GetStatusMessage(testContext, &comp)()).Should(ContainSubstring("cannot specify both \"name\" and \"labels\""))
			test.DeleteInNs(testContext, &comp, false)
			Eventually(test.GetObject(testContext, &comp)).Should(BeNil())
		})

		It("Deployment should fail when the label-based search returns more than one object", func() {
			By("Deploy another service object")
			tObj := test.LoadObject(dataDir+"serviceTest.yaml", &unstructured.Unstructured{})
			unstrObjp := tObj.(*unstructured.Unstructured)
			unstrObjp.SetName(unstrObjp.GetName() + "2")
			test.CreateObject(testContext, tObj, false, 0)
			Eventually(test.GetObject(testContext, tObj)).ShouldNot(BeNil())

			By("deploy Composable object " + "compLabels.yaml" + " with both name and labels")
			comp := test.LoadComposable(dataDir + "compLabels.yaml")
			unstrObj := unstructured.Unstructured{}
			unstrObj.UnmarshalJSON(comp.Spec.Template.Raw)
			valueFrom := fGetValueFrom(unstrObj)
			delete(valueFrom, sdk.Name)
			comp.Spec.Template.Raw, _ = unstrObj.MarshalJSON()
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())
			Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(FailedStatus))
			Expect(test.GetStatusMessage(testContext, &comp)()).Should(ContainSubstring("list object returned 2 items"))
			test.DeleteInNs(testContext, &comp, false)
			Eventually(test.GetObject(testContext, &comp)).Should(BeNil())
			test.DeleteObject(testContext, tObj, false)
			Eventually(test.GetObject(testContext, tObj)).Should(BeNil())
		})

		It("Successful deployment with correctly defined labels ", func() {
			By("deploy Composable object " + "compLabels.yaml" + " with labels")
			comp := test.LoadComposable(dataDir + "compLabels.yaml")
			unstrObj := unstructured.Unstructured{}
			unstrObj.UnmarshalJSON(comp.Spec.Template.Raw)
			valueFrom := fGetValueFrom(unstrObj)
			delete(valueFrom, sdk.Name)
			comp.Spec.Template.Raw, _ = unstrObj.MarshalJSON()
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())
			Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(OnlineStatus))
			test.DeleteInNs(testContext, &comp, false)
			Eventually(test.GetObject(testContext, &comp)).Should(BeNil())
		})

		It("Successful deployment with correctly defined labels sub-set ", func() {
			By("deploy Composable object " + "compLabels.yaml" + " with labels")
			comp := test.LoadComposable(dataDir + "compLabels.yaml")
			unstrObj := unstructured.Unstructured{}
			unstrObj.UnmarshalJSON(comp.Spec.Template.Raw)
			valueFrom := fGetValueFrom(unstrObj)
			delete(valueFrom, sdk.Name)
			lb := valueFrom[sdk.Labels].(map[string]interface{})
			delete(lb, "l1")
			comp.Spec.Template.Raw, _ = unstrObj.MarshalJSON()
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())
			Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(OnlineStatus))
			test.DeleteInNs(testContext, &comp, false)
			Eventually(test.GetObject(testContext, &comp)).Should(BeNil())
		})
	})
})

var _ = Describe("IBM cloud-operators compatibility", func() {
	dataDir := "testdata/cloud-operators-data/"
	groupVersionKind := schema.GroupVersionKind{Kind: "Service", Version: "v1alpha1", Group: "ibmcloud.ibm.com"}

	Context("create Service instance from ibmcloud.ibm.com WITHOUT external dependencies", func() {
		It("should correctly create the Service instance", func() {
			comp := test.LoadComposable(dataDir + "comp.yaml")
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			objNamespacedname := types.NamespacedName{Namespace: testContext.Namespace(), Name: "mymessagehub"}
			unstrObj := unstructured.Unstructured{}
			unstrObj.SetGroupVersionKind(groupVersionKind)
			Eventually(test.GetUnstructuredObject(testContext, objNamespacedname, &unstrObj)).Should(Succeed())
			Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(OnlineStatus))
		})

		It("should delete the Composable and Service instances", func() {
			By("Delete the Composable object")
			comp := test.LoadComposable(dataDir + "comp.yaml")
			test.DeleteInNs(testContext, &comp, false)
			Eventually(test.GetObject(testContext, &comp)).Should(BeNil())

			// TODO update for external test only
			/*
				By("Validate that the underlying object is deleted too")
				objNamespacedname := types.NamespacedName{Namespace: testContext.Namespace(), Name: "mymessagehub"}
				unstrObj := unstructured.Unstructured{}
				unstrObj.SetGroupVersionKind(groupVersionKind)
				Eventually(func() bool {
					err := test.GetUnstructuredObject(testContext, objNamespacedname, &unstrObj)()
					return errors.IsNotFound(err)
				}).Should(BeTrue())
			*/
		})
	})

	Context("create Service instance from ibmcloud.ibm.com WITH external dependencies", func() {
		var objNamespacedname types.NamespacedName

		BeforeEach(func() {
			obj := test.LoadObject(dataDir+"mysecret.yaml", &v1.Secret{})
			test.PostInNs(testContext, obj, false, 0)
			objNamespacedname = types.NamespacedName{Namespace: testContext.Namespace(), Name: "mymessagehub"}
			Eventually(test.GetObject(testContext, obj)).ShouldNot(BeNil())
		})

		AfterEach(func() {
			obj := test.LoadObject(dataDir+"mysecret.yaml", &v1.Secret{})
			test.DeleteInNs(testContext, obj, false)
		})

		It("should correctly create the Service instance according to parameters from a Secret object", func() {
			By("deploy Composable comp1.yaml")
			comp := test.LoadComposable(dataDir + "comp1.yaml")
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			By("get underlying object - Service.ibmcloud.ibm.com/v1alpha1")
			unstrObj := unstructured.Unstructured{}
			unstrObj.SetGroupVersionKind(groupVersionKind)
			Eventually(test.GetUnstructuredObject(testContext, objNamespacedname, &unstrObj)).Should(Succeed())

			By("validate service plan")
			Expect(getPlan(unstrObj.Object)).Should(Equal("standard"))

			By("Reload the Composable object")
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			By("validate that Composable object status is Online")
			Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(OnlineStatus))

			By("delete the composable object")
			test.DeleteInNs(testContext, &comp, false)
			Eventually(test.GetObject(testContext, &comp)).Should(BeNil())
		})

		It("should correctly create the Service instance according to parameters from a ConfigMap", func() {
			By("Deploy the myconfigmap  ConfigMap")
			obj := test.LoadObject(dataDir+"myconfigmap.yaml", &v1.ConfigMap{})
			test.PostInNs(testContext, obj, false, 0)
			Eventually(test.GetObject(testContext, obj)).ShouldNot(BeNil())

			By("deploy Composable comp2.yaml ")
			comp := test.LoadComposable(dataDir + "comp2.yaml")
			test.PostInNs(testContext, &comp, false, 0)
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			By("get underlying object - Service.ibmcloud.ibm.com/v1alpha1")
			unstrObj := unstructured.Unstructured{}
			unstrObj.SetGroupVersionKind(groupVersionKind)
			Eventually(test.GetUnstructuredObject(testContext, objNamespacedname, &unstrObj)).Should(Succeed())

			By("validate service plan")
			Expect(getPlan(unstrObj.Object)).Should(Equal("standard"))

			By("Reload the Composable object")
			Eventually(test.GetObject(testContext, &comp)).ShouldNot(BeNil())

			By("validate that Composable object status is Online")
			Eventually(test.GetStatusState(testContext, &comp)).Should(Equal(OnlineStatus))

			By("delete the composable object")
			test.DeleteInNs(testContext, &comp, false)
			Eventually(test.GetObject(testContext, &comp)).Should(BeNil())
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
