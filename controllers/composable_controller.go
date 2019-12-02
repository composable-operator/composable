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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	ibmcloudv1alpha1 "github.com/ibm/composable/api/v1alpha1"
	sdk "github.com/ibm/composable/sdk"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
type composableReconciler struct {
	client.Client
	log        logr.Logger
	config     *rest.Config
	scheme     *runtime.Scheme
	controller controller.Controller
}

// ManagerSettableReconciler - a Reconciler that can be added to a Manager
type ManagerSettableReconciler interface {
	reconcile.Reconciler
	SetupWithManager(mgr ctrl.Manager) error
}

var _ ManagerSettableReconciler = &composableReconciler{}

// NewReconciler ...
func NewReconciler(mgr ctrl.Manager) ManagerSettableReconciler {
	cfg := mgr.GetConfig()
	return &composableReconciler{
		Client: mgr.GetClient(),
		log:    ctrl.Log.WithName("controllers").WithName("Composable"),
		scheme: mgr.GetScheme(),
		config: cfg,
	}
}

func (r *composableReconciler) getController() controller.Controller {
	return r.controller
}

func (r *composableReconciler) setController(controller controller.Controller) {
	r.controller = controller
}

// Reconcile loop method
// +kubebuilder:rbac:groups=*,resources=*,verbs=*
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=composables/status,verbs=get;list;watch;create;update;patch;delete
func (r *composableReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.log.WithValues("composable", req.NamespacedName)

	r.log.Info("Starting reconcile loop", "request", req)
	defer r.log.Info("Finish reconcile loop", "request", req)

	// Fetch the Composable instance
	compInstance := &ibmcloudv1alpha1.Composable{}
	err := r.Get(context.TODO(), req.NamespacedName, compInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.
			// For additional cleanup logic use finalizers.
			r.log.Info("Reconciled object is not found, return", "request", req)
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		r.log.Error(err, "Get reconciled object returned", "object", req)
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
			r.log.V(1).Info("Set status", "desired status", status, "object", req)
			compInstance.Status.State = status.State
			compInstance.Status.Message = status.Message
			if err := r.Status().Update(context.Background(), compInstance); err != nil {
				r.log.Info("Error in Update", "request", err.Error())
				r.log.Error(err, "Update status", "desired status", status, "object", req, "compInstance", compInstance)
			}
		}
	}()

	// Validate the embedded template if Composable's admission control webhook is not running
	if os.Getenv("ADMISSION_CONTROL") != "true" {
		err := validateComposable(compInstance)
		if err != nil {
			status.State = FailedStatus
			status.Message = "Request is malformed and failed validation. " + err.Error()
			return ctrl.Result{}, nil
		}
	}

	// If Status is not set, set it to Pending
	if reflect.DeepEqual(compInstance.Status, ibmcloudv1alpha1.ComposableStatus{}) {
		status.State = PendingStatus
		status.Message = "Creating resource"
	}

	object, err := r.toJSONFromRaw(compInstance.Spec.Template)
	if err != nil {
		// we don't print the error, it was done in toJSONFromRaw
		status.State = FailedStatus
		status.Message = err.Error()
		// we cannot return the error, because retries do not help
		return ctrl.Result{}, nil
	}

	resource, compError := sdk.Resolve(r.Client, r.config, object, compInstance.Namespace)

	if compError != nil {
		status.Message = compError.Error.Error()
		status.State = FailedStatus
		if compError.ShouldBeReturned {
			return ctrl.Result{}, compError.Error
		}
		return ctrl.Result{}, nil

	}
	// if createUnderlyingObject faces with errors, it will update the state
	status.State = OnlineStatus
	return ctrl.Result{}, r.createUnderlyingObject(resource, compInstance, &status)
}

func (r *composableReconciler) createUnderlyingObject(resource unstructured.Unstructured,
	compInstance *ibmcloudv1alpha1.Composable,
	status *ibmcloudv1alpha1.ComposableStatus) error {

	name, err := getName(resource.Object)
	if err != nil {
		status.State = FailedStatus
		status.Message = err.Error()
		return nil
	}
	r.log.V(1).Info("Resource name is: "+name, "comName", compInstance.Name)

	namespace, err := getNamespace(resource.Object)
	if err != nil {
		status.State = FailedStatus
		status.Message = err.Error()
		return nil
	}
	r.log.V(1).Info("Resource namespace is: "+namespace, "comName", compInstance.Name)

	apiversion, ok := resource.Object[apiVersion].(string)
	if !ok {
		err := fmt.Errorf("The template has no apiVersion")
		r.log.Error(err, "", "template", resource.Object, "comName", compInstance.Name)
		status.State = FailedStatus
		status.Message = err.Error()
		return nil
	}
	r.log.V(1).Info("Resource apiversion is: "+apiversion, "comName", compInstance.Name)

	kind, ok := resource.Object[kind].(string)
	if !ok {
		err := fmt.Errorf("The template has no kind")
		r.log.Error(err, "", "template", resource.Object, "comName", compInstance.Name)
		status.State = FailedStatus
		status.Message = err.Error()
		return nil
	}
	r.log.V(1).Info("Resource kind is: " + kind)

	if err := controllerutil.SetControllerReference(compInstance, &resource, r.scheme); err != nil {
		r.log.Error(err, "SetControllerReference returned error", "resource", resource, "comName", compInstance.Name)
		status.State = FailedStatus
		status.Message = err.Error()
		return nil
	}
	underlyingObj := &unstructured.Unstructured{}
	underlyingObj.SetAPIVersion(apiversion)
	underlyingObj.SetKind(kind)
	namespaced := types.NamespacedName{Name: name, Namespace: namespace}
	r.log.Info("Get underlying resource", "resource", namespaced, "kind", kind, "apiVersion", apiversion)
	err = r.Get(context.TODO(), namespaced, underlyingObj)
	if err != nil {
		if errors.IsNotFound(err) {
			r.log.Info("Creating new underlying resource", "resource", namespaced, "kind", kind, "apiVersion", apiversion)
			err = r.Create(context.TODO(), &resource)
			if err != nil {
				r.log.Error(err, "Cannot create new resource", "resource", namespaced, "kind", kind, "apiVersion", apiversion)
				status.State = FailedStatus
				status.Message = err.Error()
				return err
			}

			// add watcher
			err = r.controller.Watch(&source.Kind{Type: underlyingObj}, &handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &ibmcloudv1alpha1.Composable{},
			})
			if err != nil {
				r.log.Error(err, "Cannot add watcher", "resource", namespaced, "kind", kind, "apiVersion", apiversion)
				status.State = FailedStatus
				status.Message = err.Error()
				return err
			}
		} else {
			r.log.Error(err, "Cannot get resource", "resource", namespaced, "kind", kind, "apiVersion", apiversion)
			status.State = FailedStatus
			status.Message = err.Error()
			return err
		}
	} else {
		// Update the found object and write the result back if there are any changes

		if !reflect.DeepEqual(resource.Object[spec], underlyingObj.Object[spec]) {
			underlyingObj.Object[spec] = resource.Object[spec]
			//r.log.Info("Updating underlying resource spec", "currentSpec", resource.Object[spec], "newSpec", underlyingObj.Object[spec], "resource", namespaced, "kind", kind, "apiVersion", apiversion)
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

// SetupWithManager adds this controller to the manager
func (r *composableReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	r.setController(c)
	// Watch for changes to Composable
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.Composable{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		//klog.Errorf("c.Watch returned %v\n", err)
		return err
	}

	return nil
}

func (r *composableReconciler) toJSONFromRaw(content *runtime.RawExtension) (interface{}, error) {
	var data interface{}
	if err := json.Unmarshal(content.Raw, &data); err != nil {
		r.log.Error(err, "json.Unmarshal error", "raw data", content.Raw)
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

func getNamespace(obj map[string]interface{}) (string, error) {
	metadata := obj[sdk.Metadata].(map[string]interface{})
	if namespace, ok := metadata[sdk.Namespace]; ok {
		return namespace.(string), nil
	}
	return "", fmt.Errorf("Failed: Template does not contain namespace")
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
