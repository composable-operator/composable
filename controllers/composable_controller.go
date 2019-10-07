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
	spec           = "spec"
	status         = "status"
	state          = "state"
	objectPrefix   = ".Object"
	transformers   = "format-transformers"
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
	err composableError
}

type composableError struct {
	error
	// TODO do we need this state separation
	isPendable bool
	// if the error is retrievable the controller will return it to the manager, and teh last will recall Reconcile again
	isRetrievable bool
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
			((status.State != OnlineStatus && !reflect.DeepEqual(status, compInstance.Status)) ||
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

	resource, compError := r.resolve(object, compInstance.Namespace)

	if compError != nil {
		status.Message = compError.Error()
		if compError.isPendable {
			status.State = PendingStatus
			return ctrl.Result{}, nil
		}
		status.State = FailedStatus
		if compError.isRetrievable {
			return ctrl.Result{}, compError.error
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

func (r *composableReconciler) resolve(object interface{}, composableNamespace string) (unstructured.Unstructured, *composableError) {
	objMap := object.(map[string]interface{})
	if _, ok := objMap[metadata]; !ok {
		err := fmt.Errorf("Failed: Template has no metadata section")
		r.log.Error(err, "", "object", objMap)
		return unstructured.Unstructured{}, &composableError{err, false, false}
	}
	// the underlying object should be created in the same namespace as the Composable object
	if metadata, ok := objMap[metadata].(map[string]interface{}); ok {
		if ns, ok := metadata[namespace]; ok {
			if composableNamespace != ns {
				err := fmt.Errorf("Failed: Template defines a wrong namespace %v", ns)
				r.log.Error(err, "", "object", objMap)
				return unstructured.Unstructured{}, &composableError{err, false, false}
			}

		} else {
			metadata[namespace] = composableNamespace
		}
	} else {
		err := fmt.Errorf("Failed: Template has an ill-defined metadata section")
		r.log.Error(err, "", "object", objMap)
		return unstructured.Unstructured{}, &composableError{err, false, false}
	}

	cache := &composableCache{objects: make(map[string]interface{})}
	obj, err := r.resolveFields(object.(map[string]interface{}), composableNamespace, cache)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	ret := unstructured.Unstructured{Object: obj.(map[string]interface{})}
	return ret, nil
}

func (r *composableReconciler) resolveFields(fields interface{}, composableNamespace string, cache *composableCache) (interface{}, *composableError) {
	switch fields.(type) {
	case map[string]interface{}:
		if fieldsOut, ok := fields.(map[string]interface{}); ok {
			for k, v := range fieldsOut {
				var newFields interface{}
				var err *composableError
				if k == getValueFrom {
					newFields, err = r.resolveValue(v, composableNamespace, cache)
					if err != nil {
						r.log.Info("resolveFields resolveValue 1", "err", err)
						return nil, err
					}
					fields = newFields
				} else if values, ok := v.(map[string]interface{}); ok {
					if value, ok := values[getValueFrom]; ok {
						if len(values) > 1 {
							err := fmt.Errorf("Failed: Template is ill-formed. GetValueFrom must be the only field in a value")
							r.log.Error(err, "resolveFields", "values", values)
							return nil, &composableError{err, false, false}
						}
						newFields, err = r.resolveValue(value, composableNamespace, cache)
					} else {
						newFields, err = r.resolveFields(values, composableNamespace, cache)
					}
					if err != nil {
						r.log.Info("resolveFields resolveValue 2", "err", err)
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

// NameMatchesResource checks if the given resource name/kind matches with API resource and its group
func NameMatchesResource(kind string, resource metav1.APIResource, resGroup string) bool {
	if strings.Contains(resource.Name, "/") {
		// subresource
		return false
	}
	lowerCaseName := strings.ToLower(kind)
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

func (r *composableReconciler) lookupAPIResource(objKind, apiVersion string) (*metav1.APIResource, *composableError) {
	//r.log.V(1).Info("lookupAPIResource", "objKind", objKind, "apiVersion", apiVersion)
	var resources []*metav1.APIResourceList
	var err error
	if len(apiVersion) > 0 {
		list, err := r.discoveryClient.ServerResourcesForGroupVersion(apiVersion)
		if err != nil {
			r.log.Error(err, "lookupAPIResource", "apiVersion", apiVersion)
			return nil, &composableError{err, false, true}
		}
		resources = []*metav1.APIResourceList{list}
		//	r.log.V(1).Info("lookupAPIResource", "list", list, "apiVersion", apiVersion)
	} else {
		resources, err = r.discoveryClient.ServerPreferredResources()
		if err != nil {
			r.log.Error(err, "lookupAPIResource ServerPreferredResources")
			return nil, &composableError{err, false, true}
		}
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
			return nil, &composableError{err, false, true}
		}

		for _, resource := range resourceList.APIResources {
			group := gv.Group
			if NameMatchesResource(objKind, resource, group) {
				if len(group) == 0 && len(apiVersion) == 0 {
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
		return nil, &composableError{err, false, false}
	}

	if targetResource != nil {
		return targetResource, nil
	}
	err = fmt.Errorf("Unable to find api resource named %q ", kind)
	r.log.Error(err, "lookupAPIResource")
	return nil, &composableError{err, false, false}
}

func (r *composableReconciler) resolveValue(value interface{}, composableNamespace string, cache *composableCache) (interface{}, *composableError) {
	//r.log.Info("resolveValue", "value", value)
	var err error
	if val, ok := value.(map[string]interface{}); ok {
		if objKind, ok := val[kind].(string); ok {
			apiversion := ""
			if apiversion, ok = val[apiVersion].(string); !ok {
				apiversion = ""
			}
			res, compErr := r.lookupAPIResource(objKind, apiversion)
			if compErr != nil {
				// We cannot resolve input object API resource, so we return error even if a default value is set.
				return nil, compErr
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
								ts := obj.(toumbstone)
								if errors.IsNotFound(ts.err.error) {
									// we have checked the object and did not fined it
									val, err1 := r.errorToDefaultValue(val, ts.err)
									return val, err1
								}
								// we should not be here
								return nil, &ts.err
							default:
								err = fmt.Errorf("wrong type of cached object %T", obj)
								r.log.Error(err, "")
								return nil, &composableError{err, false, false}
							}

						} else {
							unstrObj = unstructured.Unstructured{}
							//unstrObj.SetAPIVersion(res.Version)
							unstrObj.SetGroupVersionKind(groupVersionKind)
							r.log.V(1).Info("Get input object", "obj", objNamespacedname, "groupVersionKind", groupVersionKind)
							err = r.Get(context.TODO(), objNamespacedname, &unstrObj)
							if err != nil {
								r.log.Info("Get object returned ", "err", err, "obj", objNamespacedname)
								compErr = &composableError{err, true, true}
								cache.objects[key] = toumbstone{err: *compErr}
								if errors.IsNotFound(err) {
									return r.errorToDefaultValue(val, *compErr)
								}
								return nil, compErr
							}
							cache.objects[key] = unstrObj
						}
						j := jsonpath.New("compose")
						// add ".Object" to the path
						path = path[:1] + objectPrefix + path[1:]
						err = j.Parse(path)
						if err != nil {
							r.log.Error(err, "jsonpath.Parse", "path", path)
							return nil, &composableError{err, false, false}
						}
						j.AllowMissingKeys(false)

						fullResults, err := j.FindResults(unstrObj)
						if err != nil {
							r.log.Error(err, "FindResults", "obj", unstrObj, "path", path)
							if strings.Contains(err.Error(), "is not found") {
								val1, err1 := r.errorToDefaultValue(val, composableError{err, true, true})
								r.log.Info("resolveValue 3", "val", val1, "err", err1)
								return val, err1
							}
							return nil, &composableError{err, false, false}
						}
						// TODO check default

						iface, ok := template.PrintableValue(fullResults[0][0])
						if !ok {
							err = fmt.Errorf("can't find printable value %v ", fullResults[0][0])
							r.log.Error(err, "template.PrintableValue", "obj", unstrObj, "path", path)
							return nil, &composableError{err, false, false}
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
					err = fmt.Errorf("Failed: getValueFrom is not well-formed, 'path' is not jsonpath formated ")
					r.log.Error(err, "resolveValue", "path", path)
					return nil, &composableError{err, false, false}
				}
				err = fmt.Errorf("Failed: getValueFrom is not well-formed, 'path' is not defined ")
				r.log.Error(err, "resolveValue", "val", val)
				return nil, &composableError{err, false, false}
			}
			err = fmt.Errorf("Failed: getValueFrom is not well-formed, 'name' is not defined ")
			r.log.Error(err, "resolveValue", "val", val)
			return nil, &composableError{err, false, false}
		}
		err = fmt.Errorf("Failed: getValueFrom is not well-formed, 'kind' is not defined ")
		r.log.Error(err, "resolveValue", "val", val)
		return nil, &composableError{err, false, false}
	}
	err = fmt.Errorf("Failed: getValueFrom is not well-formed, value type is not %T ", value)
	r.log.Error(err, "resolveValue", "value", value)
	return nil, &composableError{err, false, false}
}

func (r *composableReconciler) errorToDefaultValue(val map[string]interface{}, err composableError) (interface{}, *composableError) {
	if defaultValue, ok := val[defaultValue]; ok {
		return defaultValue, nil
	}
	return nil, &err
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
