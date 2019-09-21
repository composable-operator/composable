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
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-logr/logr"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ibmcloudv1alpha1 "github.com/ibm/composable/api/v1alpha1"
)

const (
	getValueFrom   = "getValueFrom"
	defaultValue   = "defaultValue"
	name           = "name"
	path           = "path"
	namespace      = "namespace"
	metadata       = "metadata"
	kind           = "kind"
	group          = "group"
	spec           = "spec"
	status         = "status"
	state          = "state"
	objectPrefix   = ".Object"
	transformers   = "format-transformers"
	controllerName = "Compasable-controller"

	FailedStatus  = "Failed"
	PendingStatus = "Pending"
	OnlineStatus  = "Online"
)

// ComposableReconciler reconciles a Composable object
type ComposableReconciler struct {
	client.Client
	Log             logr.Logger
	DiscoveryClient discovery.DiscoveryInterface
	Config          *rest.Config
	Scheme          *runtime.Scheme
	controller      controller.Controller
}

type reconcilerWithController interface {
	reconcile.Reconciler
	getController() controller.Controller
	setController(controller controller.Controller)
}

var _ reconcilerWithController = &ComposableReconciler{}

type composableCache struct {
	objects map[string]interface{}
	// TODO replace by mem cache from K8s 1.14
	resources []*metav1.APIResourceList
}

type toumbstone struct {
	err error
}

func (r *ComposableReconciler) getController() controller.Controller {
	return r.controller
}

func (r *ComposableReconciler) setController(controller controller.Controller) {
	r.controller = controller
}

// +kubebuilder:rbac:groups=*,resources=*,verbs=*

func (r *ComposableReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("composable", req.NamespacedName)

	klog.V(5).Infoln("Start Reconcile loop")
	// Fetch the Composable instance
	instance := &ibmcloudv1alpha1.Composable{}
	err := r.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}
	klog.V(5).Infof("Reconcile loop for %s/%s", instance.Namespace, instance.Name)
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.ComposableStatus{}) {
		instance.Status = ibmcloudv1alpha1.ComposableStatus{State: PendingStatus, Message: "Creating resource"}
		if err := r.Update(context.Background(), instance); err != nil {
			return ctrl.Result{}, nil
		}
	}
	if instance.Spec.Template == nil {
		klog.V(5).Infof("The object's %s/%s spec doesn't contain `Template`", instance.Namespace, instance.Name)
		// The object's spec doesn't contain `Template`
		return ctrl.Result{}, nil
	}
	object, err := toJSONFromRaw(instance.Spec.Template)
	if err != nil {
		r.errorHandler(instance, err, PendingStatus, "", "Failed to read template data:")
		return ctrl.Result{}, err
	}
	resource, err := r.resolve(object, instance.Namespace)

	if err != nil {
		if strings.Contains(err.Error(), FailedStatus) {
			r.errorHandler(instance, err, FailedStatus, "", "")
			return ctrl.Result{}, nil
		}
		klog.Errorf("Error !!! %v\n", err)
		r.errorHandler(instance, err, PendingStatus, "", "Problem resolving template:")
		return ctrl.Result{}, err
	}

	name, err := getName(resource.Object)
	if err != nil {
		r.errorHandler(instance, err, FailedStatus, "", "")
		return ctrl.Result{}, nil

	}

	klog.V(5).Info("Resource name is: " + name)

	namespace, err := getNamespace(resource.Object)
	if err != nil {
		r.errorHandler(instance, err, FailedStatus, "", "")
		return ctrl.Result{}, nil
	}

	klog.V(5).Info("Resource namespace is: " + namespace)

	apiversion, ok := resource.Object["apiVersion"].(string)
	if !ok {
		r.errorHandler(instance, err, FailedStatus, "The template has no apiVersion", "")
		return ctrl.Result{}, nil
	}

	klog.V(5).Info("Resource apiversion is: " + apiversion)

	kind, ok := resource.Object["kind"].(string)
	if !ok {
		r.errorHandler(instance, err, FailedStatus, "The template has no kind", "")
		return ctrl.Result{}, nil
	}

	klog.V(5).Info("Resource kind is: " + kind)

	if err := controllerutil.SetControllerReference(instance, &resource, r.Scheme); err != nil {
		r.errorHandler(instance, err, PendingStatus, "", "")
		return ctrl.Result{}, err
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
				return ctrl.Result{}, nil
			}

			// add watcher
			err = r.controller.Watch(&source.Kind{Type: found}, &handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &ibmcloudv1alpha1.Composable{},
			})
			if err != nil {
				r.errorHandler(instance, err, FailedStatus, "", "")
				return ctrl.Result{}, nil
			}
		} else {
			r.errorHandler(instance, err, FailedStatus, "", "")
			return ctrl.Result{}, nil
		}
	} else {
		// Update the found object and write the result back if there are any changes
		if !reflect.DeepEqual(resource.Object[spec], found.Object[spec]) {
			found.Object[spec] = resource.Object[spec]
			klog.V(5).Infof("Updating Resource %s/%s\n", namespace, name)
			err = r.Update(context.TODO(), found)
			if err != nil {
				r.errorHandler(instance, err, FailedStatus, "", "")
				return ctrl.Result{}, nil
			}
		}
	}
	instance.Status.State = OnlineStatus
	instance.Status.Message = time.Now().Format(time.RFC850)
	err = r.Update(context.TODO(), instance)
	if err != nil {
		if strings.Contains(err.Error(), "ResourceVersion: 0") {
			// the Composable object was deleted
			return ctrl.Result{}, nil
		}
		if strings.Contains(err.Error(), "the object has been modified") {
			err = r.Get(context.TODO(), req.NamespacedName, instance)
			if err == nil {
				instance.Status.State = OnlineStatus
				instance.Status.Message = time.Now().Format(time.RFC850)
				err = r.Update(context.TODO(), instance)
				if err != nil {
					klog.Errorf("The second update status returned: %s", err.Error())
				}
				return ctrl.Result{}, err
			}
			if errors.IsNotFound(err) {
				// The Composable object was deleted.
				return ctrl.Result{}, nil
			}

		}
		klog.Errorf("Update status returned: %s", err.Error())
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil

	//return ctrl.Result{}, nil
}

func (r *ComposableReconciler) SetupWithManager(mgr ctrl.Manager) error {
	//return ctrl.NewControllerManagedBy(mgr).
	//	For(&ibmcloudv1alpha1.Composable{}).
	//	Complete(r)
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

func toJSONFromRaw(content *runtime.RawExtension) (interface{}, error) {
	var data interface{}
	if err := json.Unmarshal(content.Raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (r *ComposableReconciler) resolve(object interface{}, composableNamespace string) (unstructured.Unstructured, error) {
	// Set namespace if undefined
	objMap := object.(map[string]interface{})
	if _, ok := objMap[metadata]; !ok {
		return unstructured.Unstructured{}, fmt.Errorf("Failed: Template has no metadata section")
	}
	// the underlying object should be created in the same namespace as teh Composable object
	if metadata, ok := objMap[metadata].(map[string]interface{}); ok {
		if ns, ok := metadata[namespace]; ok {
			if composableNamespace != ns {
				return unstructured.Unstructured{}, fmt.Errorf("Failed: Template defines a wrong namespace %v", ns)
			}

		} else {
			metadata[namespace] = composableNamespace
		}
	} else {
		return unstructured.Unstructured{}, fmt.Errorf("Failed: Template has an ill-defined metadata section")
	}

	cache := &composableCache{objects: make(map[string]interface{})}
	obj, err := r.resolveFields(object.(map[string]interface{}), composableNamespace, cache)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	ret := unstructured.Unstructured{Object: obj.(map[string]interface{})}
	return ret, nil
}

func (r *ComposableReconciler) resolveFields(fields interface{}, composableNamespace string, cache *composableCache) (interface{}, error) {
	switch fields.(type) {
	case map[string]interface{}:
		if fieldsOut, ok := fields.(map[string]interface{}); ok {
			for k, v := range fieldsOut {
				var newFields interface{}
				var err error
				if k == getValueFrom {
					newFields, err = r.resolveValue(v, composableNamespace, cache)
					if err != nil {
						return nil, err
					}
					fields = newFields
				} else if values, ok := v.(map[string]interface{}); ok {
					if value, ok := values[getValueFrom]; ok {
						if len(values) > 1 {
							return nil, fmt.Errorf("Failed: Template is ill-formed. GetValueFrom must be the only field in a value.")
						}
						newFields, err = r.resolveValue(value, composableNamespace, cache)
					} else {
						newFields, err = r.resolveFields(values, composableNamespace, cache)
					}
					if err != nil {
						return nil, err
					}
					fieldsOut[k] = newFields
				} else if values, ok := v.([]interface{}); ok {
					for i, value := range values {
						newFields, err := r.resolveFields(value, composableNamespace, cache)
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
				newFields, err := r.resolveFields(value, composableNamespace, cache)
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

func (r *ComposableReconciler) GetServerPreferredResources() ([]*metav1.APIResourceList, error) {
	//client, err := discovery.NewDiscoveryClientForConfig(config)
	//if err != nil {
	//	return nil, fmt.Errorf("Error creating discovery client %v", err)
	//}
	resourceLists, err := r.DiscoveryClient.ServerPreferredResources()
	//resourceLists, err := client.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("Error listing api resources, %v", err)
	}
	return resourceLists, nil
}

func NameMatchesResource(name string, objGroup string, resource metav1.APIResource, resGroup string) bool {
	lowerCaseName := strings.ToLower(name)
	if len(objGroup) > 0 {
		if objGroup == resGroup &&
			(lowerCaseName == resource.Name ||
				lowerCaseName == resource.SingularName ||
				lowerCaseName == strings.ToLower(resource.Kind)) {
			return true
		} else {
			return false
		}
	}
	if lowerCaseName == resource.Name ||
		lowerCaseName == resource.SingularName ||
		lowerCaseName == strings.ToLower(resource.Kind) ||
		lowerCaseName == fmt.Sprintf("%s.%s", resource.Name, resGroup) {
		return true
	}
	for _, shortName := range resource.ShortNames {
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

func (r *ComposableReconciler) LookupAPIResource(objKind, objGroup string, cache *composableCache) (*metav1.APIResource, error) {
	var resources []*metav1.APIResourceList
	//if len(apiVersion) > 0  {
	//if cache.resourceMap == nil {
	//	res, err := r.discoveryClient.ServerResourcesForGroupVersion(apiVersion)
	//	if err != nil {
	//		fmt.Printf(" apiVersion error %s\n", err)
	//		return nil, err
	//	}
	//	cache.resourceMap = make(map[string]*metav1.APIResourceList)
	//	cache.resourceMap[apiVersion] = res
	//}
	//resources = []*metav1.APIResourceList{cache.resourceMap[apiVersion]}
	//fmt.Printf(" apiVersion2 %s %v \n", apiVersion, resources)
	//} else {
	if cache.resources == nil {
		klog.V(6).Infoln("Resources is nil")
		resourceList, err := r.GetServerPreferredResources()
		if err != nil {
			return nil, err
		}
		cache.resources = resourceList
	}
	resources = cache.resources
	//	}

	var targetResource *metav1.APIResource
	var matchedResources []string
	coreGroupObject := false
Loop:
	for _, resourceList := range resources {
		// The list holds the GroupVersion for its list of APIResources
		gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil {
			return nil, fmt.Errorf("Error parsing GroupVersion: %v", err)
		}

		for _, resource := range resourceList.APIResources {
			group := gv.Group
			if NameMatchesResource(objKind, objGroup, resource, group) {
				if len(group) == 0 && len(objGroup) == 0 {
					// K8s core group object
					coreGroupObject = true
					targetResource = resource.DeepCopy()
					targetResource.Group = group
					targetResource.Version = gv.Version
					coreGroupObject = true
					break Loop
				}
				if targetResource == nil {
					targetResource = resource.DeepCopy()
					targetResource.Group = group
					targetResource.Version = gv.Version
				}
				matchedResources = append(matchedResources, groupQualifiedName(resource.Name, gv.Group))
			}
		}
	}
	if !coreGroupObject && len(matchedResources) > 1 {
		return nil, fmt.Errorf("Multiple resources are matched by %q: %s. A group-qualified plural name must be provided.", kind, strings.Join(matchedResources, ", "))
	}

	if targetResource != nil {
		return targetResource, nil
	}

	return nil, fmt.Errorf("Unable to find api resource named %q.", kind)
}

func (r *ComposableReconciler) resolveValue(value interface{}, composableNamespace string, cache *composableCache) (interface{}, error) {
	if val, ok := value.(map[string]interface{}); ok {
		if objKind, ok := val[kind].(string); ok {
			objGroup := ""
			if objGroup, ok = val[group].(string); !ok {
				objGroup = ""
			}
			res, err := r.LookupAPIResource(objKind, objGroup, cache)
			if err != nil {
				// If an input object API resource is not installed, we return error even if a default value is set.
				return nil, err
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
						var unstrObj unstructured.Unstructured
						key := objectKey(objNamespacedname, groupVersionKind)
						if obj, ok := cache.objects[key]; ok {
							switch obj.(type) {
							case unstructured.Unstructured:
								unstrObj = obj.(unstructured.Unstructured)
							case toumbstone:
								// we have checked the object and did not fined it
								return errorToDefaultValue(val, obj.(toumbstone).err)
							default:
								err := fmt.Errorf("wrong type of cached object %T!", obj)
								klog.Errorf("%s", err.Error())
								return nil, err
							}

						} else {
							unstrObj = unstructured.Unstructured{}
							//unstrObj.SetAPIVersion(res.Version)
							unstrObj.SetGroupVersionKind(groupVersionKind)
							err = r.Get(context.TODO(), objNamespacedname, &unstrObj)
							if err != nil {
								klog.V(5).Infof("Get object returned %s", err.Error())
								cache.objects[key] = toumbstone{err: err}
								if errors.IsNotFound(err) {
									return errorToDefaultValue(val, err)
								}
								return nil, err
							}
							cache.objects[key] = unstrObj
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
	return "", fmt.Errorf("Failed: getValueFrom is not well-formed, its value type is %T", value)
}

func errorToDefaultValue(val map[string]interface{}, err error) (interface{}, error) {
	if defaultValue, ok := val[defaultValue]; ok {
		klog.V(5).Infof("Return default value %v for %+v due to %s \n", defaultValue, val, err.Error())
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

func getState(obj map[string]interface{}) (string, error) {
	if status, ok := obj[status].(map[string]interface{}); ok {
		if state, ok := status[state]; ok {
			return state.(string), nil
		}
		return "", fmt.Errorf("Failed: Composable doesn't contain status")
	}
	return "", fmt.Errorf("Failed: Composable doesn't contain state")
}

//func (r *ComposableReconciler) getController() controller.Controller {
//	return r.controller
//}
//
//func (r *ComposableReconciler) setController(controller controller.Controller) {
//	r.controller = controller
//}

func (r *ComposableReconciler) errorHandler(instance *ibmcloudv1alpha1.Composable, err error, status, statusMsg, errMsg string) {
	if err == nil {
		return
	}
	debug.PrintStack()
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

func objectKey(nn types.NamespacedName, gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%s/%s", nn.String(), gvk.String())
}
