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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ComposableSpec defines the desired state of Composable
type ComposableSpec struct {
	// Template defines the underlying object
	Template *runtime.RawExtension `json:"template"`
}

// ComposableStatus defines the observed state of Composable
type ComposableStatus struct {
	// State shows the composable object state
	// +optional
	// +kubebuilder:validation:Enum=Failed;Pending;Online
	State string `json:"state,omitempty"`

	// Message - provides human readable explanation of the Composable status
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=composables,scope=Namespaced,shortName=comp
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Resource Name",type=string,JSONPath=".spec.template.metadata.name"
// +kubebuilder:printcolumn:name="Resource Kind",type=string,JSONPath=".spec.template.kind"
// +kubebuilder:printcolumn:name="Resource apiVersion",type=string,JSONPath=".spec.template.apiVersion"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// Composable represents a composable resource that can wrap any resource (native kubernetes or CRDs) to allow it to be dynamically configurable
type Composable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   ComposableSpec   `json:"spec"`
	Status ComposableStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComposableList contains a list of Composable
type ComposableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Composable `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Composable{}, &ComposableList{})
}
