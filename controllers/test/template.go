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
	"io/ioutil"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/composable-operator/composable/api/v1alpha1"
)

// PostInNs the object
func PostInNs(tContext TestContext, obj client.Object, async bool, delay time.Duration) client.Object {
	obj.(metav1.ObjectMetaAccessor).GetObjectMeta().SetNamespace(tContext.Namespace())
	return CreateObject(tContext, obj, async, delay)
}

// DeletInNs the object
func DeleteInNs(tContext TestContext, obj client.Object, async bool) {
	obj.(metav1.ObjectMetaAccessor).GetObjectMeta().SetNamespace(tContext.Namespace())
	DeleteObject(tContext, obj, async)
}

// Creates the object
func CreateObject(tContext TestContext, obj client.Object, async bool, delay time.Duration) client.Object {
	done := make(chan bool)

	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		err := tContext.Client().Create(context.TODO(), obj) // FIXME proper context propagation
		if err != nil {
			panic(err)
		}
		done <- true
	}()

	if !async {
		<-done
	}
	return obj
}

// Updates the given object
func UpdateObject(tContext TestContext, obj client.Object, async bool, delay time.Duration) client.Object {
	done := make(chan bool)

	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		err := tContext.Client().Update(context.TODO(), obj)
		if err != nil {
			panic(err)
		}
		done <- true
	}()

	if !async {
		<-done
	}
	return obj
}

// DeleteObject deletes an object
func DeleteObject(tContext TestContext, obj client.Object, async bool) {
	done := make(chan bool)

	go func() {
		err := tContext.Client().Delete(context.TODO(), obj)
		if err != nil && !errors.IsNotFound(err) {
			panic(err)
		}
		done <- true
	}()

	if !async {
		<-done
	}
}

// LoadComposable loads the YAML spec into Composable object
func LoadComposable(filename string) v1alpha1.Composable {
	return *LoadObject(filename, &v1alpha1.Composable{}).(*v1alpha1.Composable)
}

// LoadObject loads the YAML spec into obj
func LoadObject(filename string, obj client.Object) client.Object {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	if err := yaml.Unmarshal(bytes, obj); err != nil {
		panic(err)
	}
	return obj
}
