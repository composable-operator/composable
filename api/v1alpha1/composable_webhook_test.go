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

package v1alpha1

import (
	"testing"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestAdmissionControl(t *testing.T) {

	embeddedGood := []byte(`{
		"apiVersion": "v1", 
		"kind": "ConfigMap",
		
		"metadata": {
		   "name": "configmapgood",
		   "namespace": "default"
		 },
		"data": {
		 "key": {
		  "getValueFrom": {
		   "kind": "Secret",
		   "name": "dallascluster",
		   "namespace": "default",
		   "path": "{.data.tls\\.key}",
		   "formatTransformers": ["Base64ToString"]
		   }
		  },
		 "dockerconfig": {
		  "getValueFrom": {
		   "kind": "Secret",
		   "name": "default-icr-io",
		   "namespace": "default",
		   "path": "{.data.\\.dockerconfigjson}"
		   }
		  }
		 }
		}`)

	embeddedBad := []byte(`{
			"kind": "ConfigMap",			
			"metadata": {
			   "name": "configmapbad",
			   "namespace": "default"
			 },
			"data": {
			 "key": {
			  "getValueFrom": {
			   "kind": "Secret",
			   "namespace": "default",
			   "formatTransformers": ["Base64ToString"]
			   }
			  },
			 "dockerconfig": {
			  "getValueFrom": {
			   "name": "default-icr-io",
			   "namespace": "default",
			   "path": "{.data.\\.dockerconfigjson}"
			   }
			  }
			 }
			}`)

	createdGood := &Composable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foogood",
			Namespace: "default",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Composable",
			APIVersion: GroupVersion.String()},
		Spec: ComposableSpec{Template: &runtime.RawExtension{Raw: embeddedGood}}}
	createdBad := &Composable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foobad",
			Namespace: "default",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Composable",
			APIVersion: GroupVersion.String()},
		Spec: ComposableSpec{Template: &runtime.RawExtension{Raw: embeddedBad}}}

	g := gomega.NewGomegaWithT(t)

	// Test validating webhook with a valid template
	g.Expect(createdGood.validateAPIVersionKind(createdGood.Spec.Template, field.NewPath("spec").Child("template"))).To(gomega.BeNil())

	_, err := createdGood.validate(createdGood.Spec.Template, field.NewPath("spec").Child("template"))
	g.Expect(hasError(err)).To(gomega.BeZero())

	// ToBeInvestigated (it fails to initiate the client)
	//g.Expect(createdGood.dryRun(m, OperationCreate)).To(gomega.BeNil())

	// Test validating webhook with an invalid template
	g.Expect(createdBad.validateAPIVersionKind(createdBad.Spec.Template, field.NewPath("spec").Child("template"))).NotTo(gomega.BeNil())
	_, err = createdBad.validate(createdBad.Spec.Template, field.NewPath("spec").Child("template"))
	g.Expect(hasError(err)).NotTo(gomega.BeZero())
}

func hasError(errList field.ErrorList) int {
	return len(errList)
}
