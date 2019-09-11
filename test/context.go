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

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestContext represents a context for test operations
type TestContext interface {
	context.Context

	// The dynamic client
	Client() client.Client

	// The object namespace being reconciled
	Namespace() string
}

type reconcileContext struct {
	context.Context
	client    client.Client
	namespace string
}

// New creates a reconcile context
func NewTestContext(client client.Client, namespace string) TestContext {
	return &reconcileContext{
		Context:   context.Background(),
		client:    client,
		namespace: namespace,
	}
}

func (c *reconcileContext) Client() client.Client {
	return c.client
}

func (c *reconcileContext) Namespace() string {
	return c.namespace
}
