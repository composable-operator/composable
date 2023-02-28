/*
Copyright 2022.

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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ibmcloudv1alpha1 "github.com/ibm/composable/api/v1alpha1"
	sdk "github.com/ibm/composable/sdk"
	"github.com/spf13/viper"
)

const (
	path           = "path"
	kind           = "kind"
	apiVersion     = "apiVersion"
	spec           = "spec"
	status         = "status"
	state          = "state"
	controllerName = "Composable-controller"

	// FailedStatus composable status
	FailedStatus = "Failed"

	// PendingStatus - indicates that the Composable object is pending for something
	PendingStatus = "Pending"

	// OnlineStatus - indicates that Composable successfully created underlying objects
	OnlineStatus = "Online"
)

// ComposableReconciler reconciles a Composable object
type ComposableReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Controller controller.Controller
	Resolver   sdk.ResolveObject
}

type ReconcilerOptions struct {
	QueriesPerSecond float32
}

// ManagerSettableReconciler - a Reconciler that can be added to a Manager
type ManagerSettableReconciler interface {
	reconcile.Reconciler
	SetupWithManager(mgr ctrl.Manager) error
}

// NewReconciler ...
func NewReconciler(mgr ctrl.Manager, opts ReconcilerOptions) ManagerSettableReconciler {
	cfg := mgr.GetConfig()
	cfg.QPS = opts.QueriesPerSecond
	return &ComposableReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Resolver: sdk.KubernetesResourceResolver{
			Client:          mgr.GetClient(),
			ResourcesClient: discovery.NewDiscoveryClientForConfigOrDie(cfg),
		},
	}
}

func (r *ComposableReconciler) getController() controller.Controller {
	return r.Controller
}

func (r *ComposableReconciler) setController(controller controller.Controller) {
	r.Controller = controller
}

// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=composables,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=composables/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=composables/finalizers,verbs=update
func (r *ComposableReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("composable", req.NamespacedName)

	logger.Info("Starting reconcile loop", "request", req)

	// Fetch the Composable instance
	compInstance := &ibmcloudv1alpha1.Composable{}
	err := r.Get(context.TODO(), req.NamespacedName, compInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.
			// For additional cleanup logic use finalizers.
			logger.Info("Reconciled object is not found, return", "request", req)
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Get reconciled object returned", "object", req)
		return ctrl.Result{}, err
	}

	status := ibmcloudv1alpha1.ComposableStatus{}
	defer func() {
		if len(status.Message) == 0 {
			status.Message = time.Now().Format(time.RFC850)
		}
		// Set Composable object Status
		if len(status.State) > 0 &&
			((status.State != OnlineStatus && !reflect.DeepEqual(status, compInstance.Status)) ||
				status.State == OnlineStatus && compInstance.Status.State != OnlineStatus) {
			logger.V(1).Info("Set status", "desired status", status, "object", req)
			compInstance.Status.State = status.State
			compInstance.Status.Message = status.Message
			if err := r.Status().Update(context.Background(), compInstance); err != nil {
				logger.Info("Error in Update", "request", err.Error())
				logger.Error(err, "Update status", "desired status", status, "object", req, "compInstance", compInstance)
			}
		}
	}()

	// If Status is not set, set it to Pending
	if reflect.DeepEqual(compInstance.Status, ibmcloudv1alpha1.ComposableStatus{}) {
		status.State = PendingStatus
		status.Message = "Creating resource"
	}

	object, err := r.toJSONFromRaw(ctx, compInstance.Spec.Template)
	if err != nil {
		// we don't print the error, it was done in toJSONFromRaw
		status.State = FailedStatus
		status.Message = err.Error()
		// we cannot return the error, because retries do not help
		return ctrl.Result{}, nil
	}

	updated, err := r.updateObjectNamespace(ctx, object, compInstance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	resource := &unstructured.Unstructured{}
	resource.Object = make(map[string]interface{})

	err = r.Resolver.ResolveObject(context.TODO(), updated, &resource.Object)

	if err != nil {
		status.Message = err.Error()
		status.State = FailedStatus
		if sdk.IsRefNotFound(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	}
	// if createUnderlyingObject faces with errors, it will update the state
	status.State = OnlineStatus
	logger.Info("Finish reconcile loop", "request", req)
	return ctrl.Result{}, r.createUnderlyingObject(ctx, *resource, compInstance, &status)
}

func (r *ComposableReconciler) updateObjectNamespace(ctx context.Context, object interface{}, composableNamespace string) (interface{}, error) {
	logger := log.FromContext(ctx)

	objMap := object.(map[string]interface{})
	if _, ok := objMap[sdk.Metadata]; !ok {
		err := fmt.Errorf("Failed: Template has no metadata section")
		return object, err
	}
	// the underlying object should be created in the same namespace as the Composable object
	if metadata, ok := objMap[sdk.Metadata].(map[string]interface{}); ok {
		if ns, ok := metadata[sdk.Namespace]; ok {
			if composableNamespace != ns {
				err := fmt.Errorf("Failed: Template defines a wrong namespace %v", ns)
				return object, err
			}
		} else {
			objMap[sdk.Metadata].(map[string]interface{})[sdk.Namespace] = composableNamespace
			logger.V(1).Info("objMap: ", "is", objMap)
			return objMap, nil
		}
	} else {
		err := fmt.Errorf("Failed: Template has an ill-defined metadata section")
		return object, err
	}
	return object, nil
}

func (r *ComposableReconciler) createUnderlyingObject(ctx context.Context, resource unstructured.Unstructured,
	compInstance *ibmcloudv1alpha1.Composable,
	status *ibmcloudv1alpha1.ComposableStatus,
) error {
	logger := log.FromContext(ctx)

	name, err := getName(resource.Object)
	if err != nil {
		status.State = FailedStatus
		status.Message = err.Error()
		return nil
	}
	logger.V(1).Info("Resource name is: "+name, "comName", compInstance.Name)

	namespace, err := sdk.GetNamespace(resource.Object)
	if err != nil {
		status.State = FailedStatus
		status.Message = err.Error()
		return nil
	}
	logger.V(1).Info("Resource namespace is: "+namespace, "comName", compInstance.Name)

	apiversion, ok := resource.Object[apiVersion].(string)
	if !ok {
		err := fmt.Errorf("The template has no apiVersion")
		logger.Error(err, "", "template", resource.Object, "comName", compInstance.Name)
		status.State = FailedStatus
		status.Message = err.Error()
		return nil
	}
	logger.V(1).Info("Resource apiversion is: "+apiversion, "comName", compInstance.Name)

	kind, ok := resource.Object[kind].(string)
	if !ok {
		err := fmt.Errorf("The template has no kind")
		logger.Error(err, "", "template", resource.Object, "comName", compInstance.Name)
		status.State = FailedStatus
		status.Message = err.Error()
		return nil
	}
	logger.V(1).Info("Resource kind is: " + kind)

	if err := controllerutil.SetControllerReference(compInstance, &resource, r.Scheme); err != nil {
		logger.Error(err, "SetControllerReference returned error", "resource", resource, "comName", compInstance.Name)
		status.State = FailedStatus
		status.Message = err.Error()
		return nil
	}
	underlyingObj := &unstructured.Unstructured{}
	underlyingObj.SetAPIVersion(apiversion)
	underlyingObj.SetKind(kind)
	namespaced := types.NamespacedName{Name: name, Namespace: namespace}
	logger.Info("Get underlying resource", "resource", namespaced, "kind", kind, "apiVersion", apiversion)
	err = r.Get(context.TODO(), namespaced, underlyingObj)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating new underlying resource", "resource", namespaced, "kind", kind, "apiVersion", apiversion)
			err = r.Create(context.TODO(), &resource)
			if err != nil {
				logger.Error(err, "Cannot create new resource", "resource", namespaced, "kind", kind, "apiVersion", apiversion)
				status.State = FailedStatus
				status.Message = err.Error()
				return err
			}

			// add watcher
			err = r.Controller.Watch(&source.Kind{Type: underlyingObj}, &handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &ibmcloudv1alpha1.Composable{},
			})
			if err != nil {
				logger.Error(err, "Cannot add watcher", "resource", namespaced, "kind", kind, "apiVersion", apiversion)
				status.State = FailedStatus
				status.Message = err.Error()
				return err
			}
		} else {
			logger.Error(err, "Cannot get resource", "resource", namespaced, "kind", kind, "apiVersion", apiversion)
			status.State = FailedStatus
			status.Message = err.Error()
			return err
		}
	} else {
		// Update the found object and write the result back if there are any changes

		if !reflect.DeepEqual(resource.Object[spec], underlyingObj.Object[spec]) {
			underlyingObj.Object[spec] = resource.Object[spec]
			// logger.Info("Updating underlying resource spec", "currentSpec", resource.Object[spec], "newSpec", underlyingObj.Object[spec], "resource", namespaced, "kind", kind, "apiVersion", apiversion)
			err = r.Update(context.TODO(), underlyingObj)
			if err != nil {

				status.State = FailedStatus
				status.Message = err.Error()
				return err
			}
		}
	}
	return nil
}

func (r *ComposableReconciler) toJSONFromRaw(ctx context.Context, content *runtime.RawExtension) (interface{}, error) {
	logger := log.FromContext(ctx)
	var data interface{}
	if err := json.Unmarshal(content.Raw, &data); err != nil {
		logger.Error(err, "json.Unmarshal error", "raw data", content.Raw)
		return nil, err
	}
	return data, nil
}

func getName(obj map[string]interface{}) (string, error) {
	metadata := obj[sdk.Metadata].(map[string]interface{})
	if name, ok := metadata[sdk.Name]; ok {
		return name.(string), nil
	}
	return "", fmt.Errorf("Failed: Template does not contain name")
}

func getState(obj map[string]interface{}) (string, error) {
	if status, ok := obj[status].(map[string]interface{}); ok {
		if state, ok := status[state]; ok {
			return state.(string), nil
		}
		return "", fmt.Errorf("Failed: Composable doesn't contain status")
	}
	return "", fmt.Errorf("Failed: Composable doesn't contain state")
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComposableReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctrl, err := ctrl.NewControllerManagedBy(mgr).
		For(&ibmcloudv1alpha1.Composable{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: viper.GetInt("max-concurrent-reconciles"),
		}).Build(r)

	r.setController(ctrl)

	return err
}
