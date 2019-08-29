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

package request

import (
	gocontext "context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Context represents a reconcile context
type Context interface {
	gocontext.Context

	// The dynamic client associated with the request
	Client() client.Client

	// The object namespace being reconciled
	Namespace() string

	// The object name being reconciled
	Name() string
}

type reconcileContext struct {
	gocontext.Context
	cl        client.Client
	namespace string
	name      string
}

// New creates a reconcile context
func New(client client.Client, request reconcile.Request) Context {
	return &reconcileContext{
		Context:   gocontext.Background(),
		cl:        client,
		namespace: request.Namespace,
		name:      request.Name,
	}
}

func (c *reconcileContext) Client() client.Client {
	return c.cl
}

func (c *reconcileContext) Namespace() string {
	return c.namespace
}

func (c *reconcileContext) Name() string {
	return c.name
}
