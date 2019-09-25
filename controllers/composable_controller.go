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
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/third_party/forked/golang/template"
	"k8s.io/client-go/util/jsonpath"
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
	apiVersion     = "apiVersion"
	group          = "group"
	spec           = "spec"
	status         = "status"
	state          = "state"
	objectPrefix   = ".Object"
	transformers   = "format-transformers"
	controllerName = "Compasable-controller"

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
	log             logr.Logger
	discoveryClient discovery.CachedDiscoveryInterface
	config          *rest.Config
	scheme          *runtime.Scheme
	controller      controller.Controller
}

// ManagerSettableReconciler - a Reconciler that can be added to a Manager
type ManagerSettableReconciler interface {
	reconcile.Reconciler
	SetupWithManager(mgr ctrl.Manager) error
}

var _ ManagerSettableReconciler = &composableReconciler{}

type composableCache struct {
	objects map[string]interface{}
}

type toumbstone struct {
	err error
}

// NewReconciler ...
func NewReconciler(mgr ctrl.Manager) ManagerSettableReconciler {
	cfg := mgr.GetConfig()
	discClient := discovery.NewDiscoveryClientForConfigOrDie(cfg)
	return &composableReconciler{
		Client:          mgr.GetClient(),
		log:             ctrl.Log.WithName("controllers").WithName("Composable"),
		discoveryClient: memory.NewMemCacheClient(discClient),
		scheme:          mgr.GetScheme(),
		config:          cfg,
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
func (r *composableReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.log.WithValues("composable", req.NamespacedName)

	r.log.Info("Starting reconcile loop", "request", req)
	defer r.log.Info("Finish reconcile loop", "request", req)

	// TODO should we use a separate go routine to invalidate it ?
	r.discoveryClient.Invalidate()

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
			((status.State != OnlineStatus && reflect.DeepEqual(status, compInstance.Status)) ||
				status.State == OnlineStatus && compInstance.Status.State != OnlineStatus) {
			r.log.V(1).Info("Set status", "desired status", status, "object", req)
			compInstance.Status.State = status.State
			compInstance.Status.Message = status.Message
			if err := r.Update(context.Background(), compInstance); err != nil {
				r.log.Error(err, "Update status", "desired status", status, "object", req, "compInstance", compInstance)
			}
		}
	}()
	// If Status is not set, set it to Pending
	if reflect.DeepEqual(compInstance.Status, ibmcloudv1alpha1.ComposableStatus{}) {
		status.State = PendingStatus
		status.Message = "Creating resource"
	}
	if compInstance.Spec.Template == nil {
		err := fmt.Errorf("The object's spec doesn't contain `Template`")
		r.log.Error(err, "object", req)
		status.State = FailedStatus
		status.Message = err.Error()
		return ctrl.Result{}, nil
	}
	object, err := r.toJSONFromRaw(compInstance.Spec.Template)
	if err != nil {
		// we don't print the error, it was done in toJSONFromRaw
		status.State = FailedStatus
		status.Message = err.Error()
		// we cannot return the error, because retries do not help
		return ctrl.Result{}, nil
	}

	resource, err := r.resolve(object, compInstance.Namespace)

	if err != nil {
		// TODO check error and return error value
		if strings.Contains(err.Error(), FailedStatus) {
			status.State = FailedStatus
			status.Message = err.Error()
			return ctrl.Result{}, nil
		}
		status.State = PendingStatus
		status.Message = err.Error()
		return ctrl.Result{}, err
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
			r.log.Info("Updating underlying resource spec", "currentSpec", resource.Object[spec], "newSpec", underlyingObj.Object[spec], "reso`urce", namespaced, "kind", kind, "apiVersion", apiversion)
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

func (r *composableReconciler) resolve(object interface{}, composableNamespace string) (unstructured.Unstructured, error) {
	objMap := object.(map[string]interface{})
	if _, ok := objMap[metadata]; !ok {
		err := fmt.Errorf("Failed: Template has no metadata section")
		r.log.Error(err, "", "object", objMap)
		return unstructured.Unstructured{}, err
	}
	// the underlying object should be created in the same namespace as the Composable object
	if metadata, ok := objMap[metadata].(map[string]interface{}); ok {
		if ns, ok := metadata[namespace]; ok {
			if composableNamespace != ns {
				err := fmt.Errorf("Failed: Template defines a wrong namespace %v", ns)
				r.log.Error(err, "", "object", objMap)
				return unstructured.Unstructured{}, err
			}

		} else {
			metadata[namespace] = composableNamespace
		}
	} else {
		err := fmt.Errorf("Failed: Template has an ill-defined metadata section")
		r.log.Error(err, "", "object", objMap)
		return unstructured.Unstructured{}, err
	}

	cache := &composableCache{objects: make(map[string]interface{})}
	obj, err := r.resolveFields(object.(map[string]interface{}), composableNamespace, cache)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	ret := unstructured.Unstructured{Object: obj.(map[string]interface{})}
	return ret, nil
}

func (r *composableReconciler) resolveFields(fields interface{}, composableNamespace string, cache *composableCache) (interface{}, error) {
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
							err := fmt.Errorf("Failed: Template is ill-formed. GetValueFrom must be the only field in a value")
							r.log.Error(err, "resolveFields", "values", values)
							return nil, err
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

// GetServerPreferredResources returns preferred API resources
func (r *composableReconciler) GetServerPreferredResources() ([]*metav1.APIResourceList, error) {
	resourceLists, err := r.discoveryClient.ServerPreferredResources()
	if err != nil {
		r.log.Error(err, "GetServerPreferredResources")
		return nil, err
	}
	return resourceLists, nil
}

// NameMatchesResource checks if the given resource name/kind and group matches with API resource and its group
func NameMatchesResource(name string, objGroup string, resource metav1.APIResource, resGroup string) bool {
	lowerCaseName := strings.ToLower(name)
	if len(objGroup) > 0 {
		if objGroup == resGroup &&
			(lowerCaseName == resource.Name ||
				lowerCaseName == resource.SingularName ||
				lowerCaseName == strings.ToLower(resource.Kind)) {
			return true
		}
		return false
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
	if len(group) == 0 {
		return name
	}
	return fmt.Sprintf("%s.%s", name, group)
}

func (r *composableReconciler) lookupAPIResource(objKind, objGroup string) (*metav1.APIResource, error) {
	var resources []*metav1.APIResourceList

	resources, err := r.GetServerPreferredResources()
	if err != nil {
		return nil, err
	}

	var targetResource *metav1.APIResource
	var matchedResources []string
	coreGroupObject := false
Loop:
	for _, resourceList := range resources {
		// The list holds the GroupVersion for its list of APIResources
		gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil {
			r.log.Error(err, "Error parsing GroupVersion", "GroupVersion", resourceList.GroupVersion)
			return nil, err
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
		err = fmt.Errorf("Multiple resources are matched by %q: %s. A group-qualified plural name must be provided ", kind, strings.Join(matchedResources, ", "))
		r.log.Error(err, "lookupAPIResource")
		return nil, err
	}

	if targetResource != nil {
		return targetResource, nil
	}
	err = fmt.Errorf("Unable to find api resource named %q ", kind)
	r.log.Error(err, "lookupAPIResource")
	return nil, err
}

func (r *composableReconciler) resolveValue(value interface{}, composableNamespace string, cache *composableCache) (interface{}, error) {
	if val, ok := value.(map[string]interface{}); ok {
		if objKind, ok := val[kind].(string); ok {
			objGroup := ""
			if objGroup, ok = val[group].(string); !ok {
				objGroup = ""
			}
			res, err := r.lookupAPIResource(objKind, objGroup)
			if err != nil {
				// We cannot resolve input object API resource, so we return error even if a default value is set.
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
								return r.errorToDefaultValue(val, obj.(toumbstone).err)
							default:
								err := fmt.Errorf("wrong type of cached object %T", obj)
								r.log.Error(err, "")
								return nil, err
							}

						} else {
							unstrObj = unstructured.Unstructured{}
							//unstrObj.SetAPIVersion(res.Version)
							unstrObj.SetGroupVersionKind(groupVersionKind)
							r.log.V(1).Info("Get input object", "obj", objNamespacedname, "groupVersionKind", groupVersionKind)
							err = r.Get(context.TODO(), objNamespacedname, &unstrObj)
							if err != nil {
								r.log.Info("Get object returned ", "err", err, "obj", objNamespacedname)
								cache.objects[key] = toumbstone{err: err}
								if errors.IsNotFound(err) {
									return r.errorToDefaultValue(val, err)
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
							r.log.Error(err, "jsonpath.Parse", "path", path)
							return nil, err
						}
						j.AllowMissingKeys(false)

						fullResults, err := j.FindResults(unstrObj)
						if err != nil {
							r.log.Error(err, "FindResults", "obj", unstrObj, "path", path)
							if strings.Contains(err.Error(), "is not found") {
								r.errorToDefaultValue(val, err)
							}
							return nil, err
						}
						// TODO check default

						iface, ok := template.PrintableValue(fullResults[0][0])
						if !ok {
							err = fmt.Errorf("can't find printable value %v ", fullResults[0][0])
							r.log.Error(err, "template.PrintableValue", "obj", unstrObj, "path", path)
							return nil, err
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
						return retVal, nil
					}
					err = fmt.Errorf("Failed: getValueFrom is not well-formed, 'path' is not jsonpath formated")
					r.log.Error(err, "resolveValue", "path", path)
					return nil, err
				}
				err = fmt.Errorf("Failed: getValueFrom is not well-formed, 'path' is not defined")
				r.log.Error(err, "resolveValue", "val", val)
				return nil, err
			}
			err = fmt.Errorf("Failed: getValueFrom is not well-formed, 'name' is not defined")
			r.log.Error(err, "resolveValue", "val", val)
			return nil, err
		}
		err := fmt.Errorf("Failed: getValueFrom is not well-formed, 'kind' is not defined")
		r.log.Error(err, "resolveValue", "val", val)
		return nil, err
	}
	err := fmt.Errorf("Failed: getValueFrom is not well-formed, value type is not %T", value)
	r.log.Error(err, "resolveValue", "value", value)
	return nil, err
}

func (r *composableReconciler) errorToDefaultValue(val map[string]interface{}, err error) (interface{}, error) {
	if defaultValue, ok := val[defaultValue]; ok {
		r.log.Info(fmt.Sprintf("Return default value %v for %+v due to %s \n", defaultValue, val, err.Error()))
		return defaultValue, nil
	}
	return nil, err
}

func getName(obj map[string]interface{}) (string, error) {
	metadata := obj[metadata].(map[string]interface{})
	if name, ok := metadata[name]; ok {
		return name.(string), nil
	}
	return "", fmt.Errorf("Failed: Template does not contain name")
}

func getNamespace(obj map[string]interface{}) (string, error) {
	metadata := obj[metadata].(map[string]interface{})
	if namespace, ok := metadata[namespace]; ok {
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

func objectKey(nn types.NamespacedName, gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%s/%s", nn.String(), gvk.String())
}
