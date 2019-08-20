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
	"reflect"
	"strings"
	"time"

	ibmcloudv1alpha1 "github.com/IBM/composable/pkg/apis/ibmcloud/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/third_party/forked/golang/template"
	"k8s.io/client-go/util/jsonpath"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	getValueFrom = "getValueFrom"
	defaultValue = "defaultValue"
	name         = "name"
	path         = "path"
	namespace    = "namespace"
	metadata     = "metadata"
	kind         = "kind"
	version      = "version"
	spec         = "spec"
	objectPrefix = ".Object"
	transformers = "format-transformers"

	FailedStatus  = "Failed"
	PendingStatus = "Pending"
	OnlineStatus  = "Online"
)

// ReconcileComposable reconciles a Composable object
type ReconcileComposable struct {
	client.Client
	config     *rest.Config
	scheme     *runtime.Scheme
	controller controller.Controller
}

type reconcilerWithController interface {
	reconcile.Reconciler
	GetController() controller.Controller
}

var _ reconcilerWithController = &ReconcileComposable{}

// Add creates a new Composable Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this ibmcloud.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcilerWithController, error) {
	r := &ReconcileComposable{Client: mgr.GetClient(), scheme: mgr.GetScheme(), config: mgr.GetConfig()}
	c, err := controller.New("composable-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	r.controller = c
	return r, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcilerWithController) error {

	c := r.GetController()
	// Watch for changes to Composable
	err := c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.Composable{}}, &handler.EnqueueRequestForObject{})
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

func toJSONFromRaw(content *runtime.RawExtension) (interface{}, error) {
	var data interface{}

	if err := json.Unmarshal(content.Raw, &data); err != nil {
		return nil, err
	}

	return data, nil
}

func (r *ReconcileComposable) resolve(object interface{}, composableNamespace string) (unstructured.Unstructured, error) {
	// Set namespace if undefined
	objMap := object.(map[string]interface{})
	if _, ok := objMap[metadata]; !ok {
		return unstructured.Unstructured{}, fmt.Errorf("Failed: Template has no metadata section")
	}
	if metadata, ok := objMap[metadata].(map[string]interface{}); ok {
		if _, ok := metadata[namespace]; !ok {
			metadata[namespace] = composableNamespace
		}
	} else {
		return unstructured.Unstructured{}, fmt.Errorf("Failed: Template has an ill-defined metadata section")
	}

	obj, err := r.resolveFields(object.(map[string]interface{}), composableNamespace, nil)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	ret := unstructured.Unstructured{Object: obj.(map[string]interface{})}
	return ret, nil
}

func (r *ReconcileComposable) resolveFields(fields interface{}, composableNamespace string, resources *[]*metav1.APIResourceList) (interface{}, error) {

	switch fields.(type) {
	case map[string]interface{}:
		if fieldsOut, ok := fields.(map[string]interface{}); ok {
			for k, v := range fieldsOut {
				var newFields interface{}
				var err error
				if k == getValueFrom {
					newFields, err = r.resolveValue(v, composableNamespace, resources)
					if err != nil {
						return nil, err
					}
					fields = newFields
				} else if values, ok := v.(map[string]interface{}); ok {
					if value, ok := values[getValueFrom]; ok {
						if len(values) > 1 {
							return nil, fmt.Errorf("Failed: Template is ill-formed. GetValueFrom must be the only field in a value")
						}
						newFields, err = r.resolveValue(value, composableNamespace, resources)
					} else {
						newFields, err = r.resolveFields(values, composableNamespace, resources)
					}
					if err != nil {
						return nil, err
					}
					fieldsOut[k] = newFields
				} else if values, ok := v.([]interface{}); ok {
					for i, value := range values {
						newFields, err := r.resolveFields(value, composableNamespace, resources)
						if err != nil {
							return nil, err
						}
						values[i] = newFields
					}
				}
			}
		}

	case []map[string]interface{}, [][]interface{}:
		if values, ok := fields.([]interface{}); ok {
			for i, value := range values {
				newFields, err := r.resolveFields(value, composableNamespace, resources)
				if err != nil {
					return nil, err
				}
				values[i] = newFields
			}
		}
	default:
		return fields, nil
	}
	return fields, nil
}

func GetServerPreferredResources(config *rest.Config) ([]*metav1.APIResourceList, error) {
	// TODO Consider using a caching scheme ala kubectl
	client, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating discovery client %v", err)
	}

	resourceLists, err := client.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("Error listing api resources, %v", err)
	}
	return resourceLists, nil
}

func NameMatchesResource(name string, apiResource metav1.APIResource, group string) bool {
	lowerCaseName := strings.ToLower(name)
	if lowerCaseName == apiResource.Name ||
		lowerCaseName == apiResource.SingularName ||
		lowerCaseName == strings.ToLower(apiResource.Kind) ||
		lowerCaseName == fmt.Sprintf("%s.%s", apiResource.Name, group) {
		return true
	}
	for _, shortName := range apiResource.ShortNames {
		if lowerCaseName == strings.ToLower(shortName) {
			return true
		}
	}

	return false
}

func groupQualifiedName(name, group string) string {
	apiResource := metav1.APIResource{
		Name:  name,
		Group: group,
	}

	return GroupQualifiedName(apiResource)
}

func GroupQualifiedName(apiResource metav1.APIResource) string {
	if len(apiResource.Group) == 0 {
		return apiResource.Name
	}
	return fmt.Sprintf("%s.%s", apiResource.Name, apiResource.Group)
}

func (r *ReconcileComposable) LookupAPIResource(key, targetVersion string, resources *[]*metav1.APIResourceList) (*metav1.APIResource, error) {
	if resources == nil {
		resourceList, err := GetServerPreferredResources(r.config)
		if err != nil {
			return nil, err
		}
		resources = &resourceList
	}

	var targetResource *metav1.APIResource
	var matchedResources []string
	for _, resourceList := range *resources {
		// The list holds the GroupVersion for its list of APIResources
		gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil {
			return nil, fmt.Errorf("Error parsing GroupVersion: %v", err)
		}
		if len(targetVersion) > 0 && gv.Version != targetVersion {
			continue
		}
		for _, resource := range resourceList.APIResources {
			group := gv.Group
			if NameMatchesResource(key, resource, group) {
				if targetResource == nil {
					targetResource = resource.DeepCopy()
					targetResource.Group = group
					targetResource.Version = gv.Version
				}
				matchedResources = append(matchedResources, groupQualifiedName(resource.Name, gv.Group))
			}
		}

	}
	if len(matchedResources) > 1 {
		return nil, fmt.Errorf("Multiple resources are matched by %q: %s. A group-qualified plural name must be provided.", key, strings.Join(matchedResources, ", "))
	}

	if targetResource != nil {
		return targetResource, nil
	}

	return nil, fmt.Errorf("Unable to find api resource named %q.", key)
}

func (r *ReconcileComposable) resolveValue(value interface{}, composableNamespace string, resources *[]*metav1.APIResourceList) (interface{}, error) {
	if val, ok := value.(map[string]interface{}); ok {
		if kind, ok := val[kind].(string); ok {
			vers := ""
			if vers, ok = val[version].(string); ok {
			}
			res, err := r.LookupAPIResource(kind, vers, resources)
			if err != nil {
				return errorToDefaultValue(val, err)
			}
			if name, ok := val[name].(string); ok {
				if path, ok := val[path].(string); ok {
					if strings.HasPrefix(path, "{.") {
						var objNamespacedname types.NamespacedName
						if res.Namespaced {
							namespace, ok := val[namespace].(string)
							if !ok {
								namespace = composableNamespace
							}
							objNamespacedname = types.NamespacedName{Namespace: namespace, Name: name}
						} else {
							objNamespacedname = types.NamespacedName{Name: name}
						}

						groupVersionKind := schema.GroupVersionKind{Kind: res.Kind, Version: res.Version, Group: res.Group}
						unstrObj := unstructured.Unstructured{}
						unstrObj.SetAPIVersion(res.Version)
						unstrObj.SetGroupVersionKind(groupVersionKind)
						err = r.Get(context.TODO(), objNamespacedname, &unstrObj)
						if err != nil {
							if errors.IsNotFound(err) {
								return errorToDefaultValue(val, err)
							}
							return nil, err
						}
						j := jsonpath.New("compose")
						// add ".Object" to the path
						path = path[:1] + objectPrefix + path[1:]
						err = j.Parse(path)
						if err != nil {
							klog.Errorf("jsonpath is %s, error is %s", path, err.Error())
							return nil, err
						}
						j.AllowMissingKeys(false)

						fullResults, err := j.FindResults(unstrObj)
						if err != nil {
							if strings.Contains(err.Error(), "is not found") {
								errorToDefaultValue(val, err)
							}
							return nil, err
						}
						// TODO check default

						iface, ok := template.PrintableValue(fullResults[0][0])
						if !ok {
							return nil, fmt.Errorf("can't print type %s", fullResults[0][0])
						}

						var retVal interface{}
						if transformers, ok := val[transformers].([]interface{}); ok && len(transformers) > 0 {
							transformNames := make([]string, 0, len(transformers))
							for _, v := range transformers {
								if name, ok := v.(string); ok {
									transformNames = append(transformNames, name)
								}
							}
							retVal, err = CompoundTransformerNames(iface, transformNames...)
						} else {
							retVal = iface
						}
						klog.V(5).Infof("resolveValue returned %v [%T]\n", retVal, retVal)
						return retVal, nil
					}
					return nil, fmt.Errorf("Failed: getValueFrom is not well-formed, 'path' is not jsonpath formated")

				}
				return nil, fmt.Errorf("Failed: getValueFrom is not well-formed, 'path' is not defined")
			}
			return nil, fmt.Errorf("Failed: getValueFrom is not well-formed, 'name' is not defined")

		}
		return "", fmt.Errorf("Failed: getValueFrom is not well-formed, 'kind' is not defined")
	}
	return "", fmt.Errorf("Failed: getValueFrom is not well-formed")
}

func errorToDefaultValue(val map[string]interface{}, err error) (interface{}, error) {
	if defaultValue, ok := val[defaultValue]; ok {
		klog.V(5).Infof("Return default value %v\n", defaultValue)
		return defaultValue, nil
	}
	return nil, err
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

func (r *ReconcileComposable) GetController() controller.Controller {
	return r.controller
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
		instance.Status = ibmcloudv1alpha1.ComposableStatus{State: PendingStatus, Message: "Creating resource"}
		if err := r.Update(context.Background(), instance); err != nil {
			return reconcile.Result{}, nil
		}
	}
	if instance.Spec.Template == nil {
		// The object's spec doesn't contain `Template`
		return reconcile.Result{}, nil
	}
	object, err := toJSONFromRaw(instance.Spec.Template)
	if err != nil {
		r.errorHandler(instance, err, PendingStatus, "", "Failed to read template data:")
		return reconcile.Result{}, err
	}

	resource, err := r.resolve(object, instance.Namespace)
	if err != nil {
		if strings.Contains(err.Error(), FailedStatus) {
			r.errorHandler(instance, err, FailedStatus, "", "")
			return reconcile.Result{}, nil
		}
		r.errorHandler(instance, err, PendingStatus, "", "Problem resolving template:")
		return reconcile.Result{}, err
	}

	name, err := getName(resource.Object)
	if err != nil {
		r.errorHandler(instance, err, FailedStatus, "", "")
		return reconcile.Result{}, nil

	}

	klog.V(5).Info("Resource name is: " + name)

	namespace, err := getNamespace(resource.Object)
	if err != nil {
		r.errorHandler(instance, err, FailedStatus, "", "")
		return reconcile.Result{}, nil
	}

	klog.V(5).Info("Resource namespace is: " + namespace)

	apiversion, ok := resource.Object["apiVersion"].(string)
	if !ok {
		r.errorHandler(instance, err, FailedStatus, "The template has no apiVersion", "")
		return reconcile.Result{}, nil
	}

	klog.V(5).Info("Resource apiversion is: " + apiversion)

	kind, ok := resource.Object["kind"].(string)
	if !ok {
		r.errorHandler(instance, err, FailedStatus, "The template has no kind", "")
		return reconcile.Result{}, nil
	}

	klog.V(5).Info("Resource kind is: " + kind)

	if err := controllerutil.SetControllerReference(instance, &resource, r.scheme); err != nil {
		r.errorHandler(instance, err, PendingStatus, "", "")
		return reconcile.Result{}, err
	}
	found := &unstructured.Unstructured{}
	found.SetAPIVersion(apiversion)
	found.SetKind(kind)
	err = r.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(5).Infof("Creating resource %s/%s\n", namespace, name)
			err = r.Create(context.TODO(), &resource)
			if err != nil {
				klog.Errorf("Creation of resource %s/%s returned error: %s\n", namespace, name, err.Error())
				if instance.Status.State != FailedStatus {
					r.errorHandler(instance, err, FailedStatus, "Failed", "")
				}
				return reconcile.Result{}, nil
			}

			// add watcher
			err = r.controller.Watch(&source.Kind{Type: found}, &handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &ibmcloudv1alpha1.Composable{},
			})
			if err != nil {
				r.errorHandler(instance, err, FailedStatus, "", "")
				return reconcile.Result{}, nil
			}
		} else {
			r.errorHandler(instance, err, FailedStatus, "", "")
			return reconcile.Result{}, nil
		}
	} else {
		// Update the found object and write the result back if there are any changes
		if !reflect.DeepEqual(resource.Object[spec], found.Object[spec]) {
			found.Object[spec] = resource.Object[spec]
			klog.V(5).Infof("Updating Resource %s/%s\n", namespace, name)
			err = r.Update(context.TODO(), found)
			if err != nil {
				r.errorHandler(instance, err, FailedStatus, "", "")
				return reconcile.Result{}, nil
			}
		}
	}
	instance.Status.State = OnlineStatus
	instance.Status.Message = time.Now().Format(time.RFC850)
	err = r.Update(context.TODO(), instance)
	if err != nil {
		r.errorHandler(instance, err, FailedStatus, "", "")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileComposable) errorHandler(instance *ibmcloudv1alpha1.Composable, err error, status, statusMsg, errMsg string) {
	if err == nil {
		return
	}
	klog.Errorf("error: %v, message %s", err, errMsg)
	instance.Status.State = status
	if statusMsg != "" {
		instance.Status.Message = statusMsg
	} else {
		instance.Status.Message = err.Error()
	}
	er := r.Update(context.TODO(), instance)
	if er != nil {
		klog.Errorf("Embedded error of updating %s %s/%s, error is %s \n", instance.Kind, instance.Name, instance.Namespace, err.Error())
	}
}
