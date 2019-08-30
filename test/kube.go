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
	"strings"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc" //
	"k8s.io/client-go/rest"
)

// SetupKubeOrDie setups Kube for testing
func SetupKubeOrDie(restCfg *rest.Config, stem string) string {
	clientset := GetClientsetOrDie(restCfg)
	namespace := CreateNamespaceOrDie(clientset.CoreV1().Namespaces(), stem)

	return namespace
}

// GetClientsetOrDie gets a Kube clientset for KUBECONFIG
func GetClientsetOrDie(restCfg *rest.Config) *kubernetes.Clientset {
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		panic(err)
	}
	return clientset
}

// CreateNamespaceOrDie creates a new unique namespace from stem
func CreateNamespaceOrDie(namespaces corev1.NamespaceInterface, stem string) string {
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: stem}}
	ns, err := namespaces.Create(ns)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			panic(err)
		}
	}
	return ns.Name
}
