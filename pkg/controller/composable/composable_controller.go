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

package composable

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	ibmcloudv1alpha1 "github.ibm.com/seed/composable/pkg/apis/ibmcloud/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const getValueFrom = "getValueFrom"

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Composable Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this ibmcloud.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileComposable{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("composable-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	r.(*ReconcileComposable).controller = c

	// Watch for changes to Composable
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.Composable{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODOMV: Replace here with type created
	// TODO(user): Modify this to be the types you create
	// Uncomment watch a Deployment created by Composable - change this for objects you create
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &ibmcloudv1alpha1.Composable{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileComposable{}

// ReconcileComposable reconciles a Composable object
type ReconcileComposable struct {
	client.Client
	scheme     *runtime.Scheme
	controller controller.Controller
}

func toJSONFromRaw(content *runtime.RawExtension) (interface{}, error) {
	var data interface{}

	if err := json.Unmarshal(content.Raw, &data); err != nil {
		return nil, err
	}

	return data, nil
}

func resolve(r *ReconcileComposable, object interface{}, composableNamespace string) (unstructured.Unstructured, error) {
	// Set namespace if undefined
	objMap := object.(map[string]interface{})
	if _, ok := objMap["metadata"]; !ok {
		return unstructured.Unstructured{}, fmt.Errorf("Failed: Template has no metadata section")
	}
	if metadata, ok := objMap["metadata"].(map[string]interface{}); ok {
		if _, ok := metadata["namespace"]; !ok {
			metadata["namespace"] = composableNamespace
		}
	} else {
		return unstructured.Unstructured{}, fmt.Errorf("Failed: Template has an ill-defined metadata section")
	}

	obj, err := resolveFields(r, object.(map[string]interface{}), composableNamespace)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	ret := unstructured.Unstructured{
		Object: obj,
	}
	return ret, nil
}

func resolveFields(r *ReconcileComposable, fields map[string]interface{}, composableNamespace string) (map[string]interface{}, error) {
	for k, v := range fields {
		if values, ok := v.(map[string]interface{}); ok {
			if value, ok := values[getValueFrom]; ok {
				if len(values) > 1 {
					return nil, fmt.Errorf("Failed: Template is ill-formed. GetValueFrom must be the only field in a value")
				}
				resolvedValue, err := resolveValue(r, value, composableNamespace)
				if err != nil {
					return nil, err
				}
				fields[k] = resolvedValue
			} else {
				newFields, err := resolveFields(r, values, composableNamespace)
				if err != nil {
					return nil, err
				}
				fields[k] = newFields
			}
		}
	}
	return fields, nil
}

func resolveValue(r *ReconcileComposable, value interface{}, composableNamespace string) (string, error) {
	if val, ok := value.(map[string]interface{}); ok {

		// SecretRef
		if obj, ok := val["secretKeyRef"]; ok {
			if objmap, ok := obj.(map[string]interface{}); ok {
				name, ok := objmap["name"].(string)
				if !ok {
					return "", fmt.Errorf("Failed: GetValueFrom is not well-formed, missing name for secretKeyRef")
				}
				key, ok := objmap["key"].(string)
				if !ok {
					return "", fmt.Errorf("Failed: GetValueFrom is not well-formed, missing key for secretKeyRef")
				}
				namespace, ok := objmap["namespace"].(string)
				if !ok {
					namespace = composableNamespace
				}
				secretNamespacedname := types.NamespacedName{Namespace: namespace, Name: name}
				secret := &v1.Secret{}

				err := r.Get(context.TODO(), secretNamespacedname, secret)
				if err != nil {
					return "", err
				}
				secretData := secret.Data[key]
				return string(secretData), nil

			}
			return "", fmt.Errorf("Failed: GetValueFrom is not well-formed, secretKeyRef is not a map")

			// ConfigMapRef
		} else if obj, ok := val["configMapRef"]; ok {
			if objmap, ok := obj.(map[string]interface{}); ok {
				name, ok := objmap["name"].(string)
				if !ok {
					return "", fmt.Errorf("Failed: GetValueFrom is not well-formed, missing name for configMapRef")
				}
				key, ok := objmap["key"].(string)
				if !ok {
					return "", fmt.Errorf("Failed: GetValueFrom is not well-formed, missing key for configMapRef")
				}
				namespace, ok := objmap["namespace"].(string)
				if !ok {
					namespace = composableNamespace
				}
				configMapNamespacedname := types.NamespacedName{Namespace: namespace, Name: name}
				configMap := &v1.ConfigMap{}

				err := r.Get(context.TODO(), configMapNamespacedname, configMap)
				if err != nil {
					return "", err
				}
				configMapData := configMap.Data[key]
				return string(configMapData), nil
			}
			return "", fmt.Errorf("Failed: GetValueFrom is not well-formed, configMapRef is not a map")
		}
	}
	return "", fmt.Errorf("Failed: GetValueFrom is not well-formed")
}

func getName(obj map[string]interface{}) (string, error) {
	metadata := obj["metadata"].(map[string]interface{})
	if name, ok := metadata["name"]; ok {
		return name.(string), nil
	}
	return "", fmt.Errorf("Failed: Template does not contain name")
}

func getNamespace(obj map[string]interface{}) (string, error) {
	metadata := obj["metadata"].(map[string]interface{})
	if namespace, ok := metadata["namespace"]; ok {
		return namespace.(string), nil
	}
	return "", fmt.Errorf("Failed: Template does not contain namespace")
}

// Reconcile reads that state of the cluster for a Composable object and makes changes based on the state read
// and what is in the Composable.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=composables,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileComposable) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Composable instance
	instance := &ibmcloudv1alpha1.Composable{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.ComposableStatus{}) {
		instance.Status = ibmcloudv1alpha1.ComposableStatus{State: "Pending", Message: "Creating resource"}
		if err := r.Update(context.Background(), instance); err != nil {
			return reconcile.Result{}, nil
		}
	}

	object, err := toJSONFromRaw(instance.Spec.Template)
	if err != nil {
		log.Printf("Failed to read template data: %s\n" + err.Error())
		instance.Status.State = "Pending"
		instance.Status.Message = err.Error()
		r.Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}

	resource, err := resolve(r, object, instance.Namespace)
	if err != nil {
		if strings.Contains(err.Error(), "Failed") {
			log.Printf(err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = err.Error()
			r.Update(context.TODO(), instance)
			return reconcile.Result{}, nil
		}
		log.Printf("Problem resolving template: %s\n", err.Error())
		instance.Status.State = "Pending"
		instance.Status.Message = err.Error()
		r.Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}
	log.Println(resource)

	name, err := getName(resource.Object)
	if err != nil {
		log.Printf(err.Error())
		instance.Status.State = "Failed"
		instance.Status.Message = err.Error()
		r.Update(context.TODO(), instance)
		return reconcile.Result{}, nil

	}

	log.Println("Resource name is: " + name)

	namespace, err := getNamespace(resource.Object)
	if err != nil {
		log.Printf(err.Error())
		instance.Status.State = "Failed"
		instance.Status.Message = err.Error()
		r.Update(context.TODO(), instance)
		return reconcile.Result{}, nil
	}

	log.Println("Resource namespace is: " + namespace)

	apiversion, ok := resource.Object["apiVersion"].(string)
	if !ok {
		instance.Status.State = "Failed"
		instance.Status.Message = "The template has no apiVersion"
		r.Update(context.TODO(), instance)
		return reconcile.Result{}, nil
	}

	log.Println("Resource apiversion is: " + apiversion)

	kind, ok := resource.Object["kind"].(string)
	if !ok {
		instance.Status.State = "Failed"
		instance.Status.Message = "The template has no kind"
		r.Update(context.TODO(), instance)
		return reconcile.Result{}, nil
	}

	log.Println("Resource kind is: " + kind)

	if err := controllerutil.SetControllerReference(instance, &resource, r.scheme); err != nil {
		log.Println(err.Error())
		instance.Status.State = "Pending"
		instance.Status.Message = err.Error()
		r.Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}

	found := &unstructured.Unstructured{}
	found.SetAPIVersion(apiversion)
	found.SetKind(kind)

	err = r.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && strings.Contains(err.Error(), "not found") {
		log.Println(err.Error())
		log.Printf("Creating resource %s/%s\n", namespace, name)
		err = r.Create(context.TODO(), &resource)
		if err != nil {
			log.Printf(err.Error())
			if instance.Status.State != "Failed" {
				instance.Status.State = "Failed"
				instance.Status.Message = err.Error()
				r.Update(context.TODO(), instance)
			}
			return reconcile.Result{}, nil
		}

		// add watcher
		err = r.controller.Watch(&source.Kind{Type: found}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &ibmcloudv1alpha1.Composable{},
		})
		if err != nil {
			log.Printf(err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = err.Error()
			r.Update(context.TODO(), instance)
			return reconcile.Result{}, nil
		}
	} else if err != nil {
		log.Printf(err.Error())
		instance.Status.State = "Failed"
		instance.Status.Message = err.Error()
		r.Update(context.TODO(), instance)
		return reconcile.Result{}, nil
	}

	// Update the found object and write the result back if there are any changes
	if !reflect.DeepEqual(resource.Object["Spec"], found.Object["Spec"]) {
		found.Object["Spec"] = resource.Object["Spec"]
		log.Printf("Updating Resource %s/%s\n", namespace, name)
		err = r.Update(context.TODO(), found)
		if err != nil {
			log.Printf(err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = err.Error()
			r.Update(context.TODO(), instance)
			return reconcile.Result{}, nil
		}
	}

	instance.Status.State = "Online"
	instance.Status.Message = time.Now().Format(time.RFC850)
	r.Update(context.TODO(), instance)
	return reconcile.Result{}, nil
}
