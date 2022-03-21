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

package test

import (
	"context"

	"github.com/ibm/composable/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetObject gets the object from the store
func GetObject(tContext TestContext, obj client.Object) func() client.Object {
	return func() client.Object {
		key := client.ObjectKeyFromObject(obj)
		if err := tContext.Client().Get(context.TODO(), key, obj); err != nil {
			return nil
		}
		return obj
	}
}

// GetObject gets the object from the store
func GetUnstructuredObject(tContext TestContext, namespacedname types.NamespacedName, obj *unstructured.Unstructured) func() error {
	return func() error {
		client := tContext.Client()
		return client.Get(context.TODO(), namespacedname, obj)
	}
}

// GetStatusState returns the status state of a Composable object
func GetStatusState(tContext TestContext, comp *v1alpha1.Composable) func() string {
	return func() string {
		if obj := GetObject(tContext, comp)(); comp != nil {
			c := obj.(*v1alpha1.Composable)
			return c.Status.State
		}
		return ""
	}
}

// GetStatusMessage returns the status message of a Composable object
func GetStatusMessage(tContext TestContext, comp *v1alpha1.Composable) func() string {
	return func() string {
		if obj := GetObject(tContext, comp)(); comp != nil {
			c := obj.(*v1alpha1.Composable)
			return c.Status.Message
		}
		return ""
	}
}
