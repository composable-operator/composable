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
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Load all client auth plugins for GCP, Azure, Openstack, etc
)

// CreateNamespaceOrDie creates a new namespace
func CreateNamespaceOrDie(ctx context.Context, tContext TestContext, name string) error {
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	err := tContext.Client().Create(ctx, ns)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return nil
		}
	}
	return nil
}
