package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/third_party/forked/golang/template"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logf = log.Log.WithName("composable-sdk")

// String constants
const (
	Metadata     = "metadata"
	Namespace    = "namespace"
	GetValueFrom = "getValueFrom"
	kind         = "kind"
	apiVersion   = "apiVersion"
	path         = "path"
	Name         = "name"
	Labels       = "labels"
	Transformers = "format-transformers"
	defaultValue = "defaultValue"
	objectPrefix = ".Object"
)

// KubernetesResourceResolver implements the ResolveObject interface
type KubernetesResourceResolver struct {
	Client          client.Client
	ResourcesClient discovery.ServerResourcesInterface
}

// ResolveObject resolves the object into resolved
func (k KubernetesResourceResolver) ResolveObject(ctx context.Context, object interface{}, resolved interface{}) error {
	var objectMap map[string]interface{}
	inrec, err := json.Marshal(object)
	if err != nil {
		return err
	}

	err = json.Unmarshal(inrec, &objectMap)
	if err != nil {
		return err
	}

	namespace, err := GetNamespace(objectMap)
	if err != nil {
		return err
	}

	unstructured, comperr := Resolve(ctx, k.Client, k.ResourcesClient, objectMap, namespace)
	if comperr != nil {
		return comperr.Error
	}

	inrec, err = json.Marshal(unstructured.Object)
	if err != nil {
		return err
	}

	err = json.Unmarshal(inrec, &resolved)
	if err != nil {
		return err
	}

	return nil
}

// Resolve resolves an object and returns an Unstructured
func Resolve(ctx context.Context, r client.Client, discoveryClient discovery.ServerResourcesInterface, object interface{}, composableNamespace string) (unstructured.Unstructured, *ComposableError) {
	objMap := object.(map[string]interface{})
	if _, ok := objMap[Metadata]; !ok {
		err := fmt.Errorf("Failed: Template has no metadata section")
		logf.Error(err, "", "object", objMap)
		return unstructured.Unstructured{}, &ComposableError{err, false}
	}
	// the underlying object should be created in the same namespace as the Composable object
	if metadata, ok := objMap[Metadata].(map[string]interface{}); ok {
		if ns, ok := metadata[Namespace]; ok {
			if composableNamespace != ns {
				err := fmt.Errorf("Failed: Template defines a wrong namespace %v", ns)
				logf.Error(err, "", "object", objMap)
				return unstructured.Unstructured{}, &ComposableError{err, false}
			}

		} else {
			metadata[Namespace] = composableNamespace
		}
	} else {
		err := fmt.Errorf("Failed: Template has an ill-defined metadata section")
		logf.Error(err, "", "object", objMap)
		return unstructured.Unstructured{}, &ComposableError{err, false}
	}

	cache := &ComposableCache{objects: make(map[string]interface{})}
	obj, err := resolveFields(r, object.(map[string]interface{}), composableNamespace, cache, discoveryClient)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	ret := unstructured.Unstructured{Object: obj.(map[string]interface{})}
	return ret, nil
}

func resolveFields(r client.Client, fields interface{}, composableNamespace string, cache *ComposableCache, discoveryClient discovery.ServerResourcesInterface) (interface{}, *ComposableError) {
	switch fields.(type) {
	case map[string]interface{}:
		if fieldsOut, ok := fields.(map[string]interface{}); ok {
			for k, v := range fieldsOut {
				var newFields interface{}
				var err *ComposableError
				if k == GetValueFrom {
					newFields, err = resolveValue(r, v, composableNamespace, cache, discoveryClient)
					if err != nil {
						logf.Info("resolveFields resolveValue 1", "err", err)
						return nil, err
					}
					fields = newFields
				} else if values, ok := v.(map[string]interface{}); ok {
					if value, ok := values[GetValueFrom]; ok {
						if len(values) > 1 {
							err := fmt.Errorf("Failed: Template is ill-formed. GetValueFrom must be the only field in a value")
							logf.Error(err, "resolveFields", "values", values)
							return nil, &ComposableError{err, false}
						}
						newFields, err = resolveValue(r, value, composableNamespace, cache, discoveryClient)
					} else {
						newFields, err = resolveFields(r, values, composableNamespace, cache, discoveryClient)
					}
					if err != nil {
						logf.Info("resolveFields resolveValue 2", "err", err)
						return nil, err
					}
					fieldsOut[k] = newFields
				} else if values, ok := v.([]interface{}); ok {
					for i, value := range values {
						newFields, err := resolveFields(r, value, composableNamespace, cache, discoveryClient)
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
				newFields, err := resolveFields(r, value, composableNamespace, cache, discoveryClient)
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

func lookupAPIResource(discoveryClient discovery.ServerResourcesInterface, objKind, apiVersion string) (*metav1.APIResource, *ComposableError) {
	//r.log.V(1).Info("lookupAPIResource", "objKind", objKind, "apiVersion", apiVersion)
	var resources []*metav1.APIResourceList
	var err error
	if len(apiVersion) > 0 {
		list, err := discoveryClient.ServerResourcesForGroupVersion(apiVersion)
		if err != nil {
			logf.Error(err, "lookupAPIResource", "apiVersion", apiVersion)
			return nil, &ComposableError{err, true}
		}
		resources = []*metav1.APIResourceList{list}
		//	r.log.V(1).Info("lookupAPIResource", "list", list, "apiVersion", apiVersion)
	} else {
		resources, err = discoveryClient.ServerPreferredResources()
		if err != nil {
			logf.Error(err, "lookupAPIResource ServerPreferredResources")
			return nil, &ComposableError{err, true}
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
			logf.Error(err, "Error parsing GroupVersion", "GroupVersion", resourceList.GroupVersion)
			return nil, &ComposableError{err, true}
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
		logf.Error(err, "lookupAPIResource")
		return nil, &ComposableError{err, false}
	}

	if targetResource != nil {
		return targetResource, nil
	}
	err = fmt.Errorf("Unable to find api resource named %q ", kind)
	logf.Error(err, "lookupAPIResource")
	return nil, &ComposableError{err, false}
}

func resolveValue(r client.Client, value interface{}, composableNamespace string, cache *ComposableCache, discoveryClient discovery.ServerResourcesInterface) (interface{}, *ComposableError) {
	//r.log.Info("resolveValue", "value", value)
	var err error
	if val, ok := value.(map[string]interface{}); ok {
		if objKind, ok := val[kind].(string); ok {
			apiversion := ""
			if apiversion, ok = val[apiVersion].(string); !ok {
				apiversion = ""
			}
			if path, ok := val[path].(string); ok {
				if strings.HasPrefix(path, "{.") {

					unstrObj, compErr := getInputObject(r, val, objKind, apiversion, composableNamespace, cache, discoveryClient)
					if compErr != nil {
						if errors.IsNotFound(compErr.Error) {
							// we have checked the object and did not fined it
							val, err1 := errorToDefaultValue(val, *compErr)
							return val, err1
						}
						// we should not be here
						return nil, compErr
					}
					return resolveValue2(val, *unstrObj, path)
				}
				err = fmt.Errorf("Failed: getValueFrom is not well-formed, 'path' is not jsonpath formated ")
				logf.Error(err, "resolveValue", "path", path)
				return nil, &ComposableError{err, false}
			}
			err = fmt.Errorf("Failed: getValueFrom is not well-formed, 'path' is not defined ")
			logf.Error(err, "resolveValue", "val", val)
			return nil, &ComposableError{err, false}
		}
		err = fmt.Errorf("Failed: getValueFrom is not well-formed, 'kind' is not defined ")
		logf.Error(err, "resolveValue", "val", val)
		return nil, &ComposableError{err, false}
	}
	err = fmt.Errorf("Failed: getValueFrom is not well-formed, value type is not %T ", value)
	logf.Error(err, "resolveValue", "value", value)
	return nil, &ComposableError{err, false}
}

func getInputObject(r client.Client, val map[string]interface{}, objKind, apiversion, composableNamespace string, cache *ComposableCache, discoveryClient discovery.ServerResourcesInterface) (*unstructured.Unstructured, *ComposableError) {
	res, compErr := lookupAPIResource(discoveryClient, objKind, apiversion)
	if compErr != nil {
		// We cannot resolve input object API resource, so we return error even if a default value is set.
		return nil, compErr
	}
	groupVersionKind := schema.GroupVersionKind{Kind: res.Kind, Version: res.Version, Group: res.Group}
	var ns string
	var ok bool
	if res.Namespaced {
		ns, ok = val[Namespace].(string)
		if !ok {
			ns = composableNamespace
		}
	}
	var err error
	name, nameOK := val[Name].(string)
	intLabels, labelsOK := val[Labels].(map[string]interface{})
	if nameOK == labelsOK { // only one of them should be defined
		if nameOK && labelsOK {
			err = fmt.Errorf("Failed: getValueFrom is not well-formed, both 'name' and 'labels' cannot be defined ")
		} else {
			err = fmt.Errorf("Failed: getValueFrom is not well-formed, neither 'name' nor 'labels' are not defined  ")
		}
		logf.Error(err, "getInputObject", "val", val)
		return nil, &ComposableError{err, false}
	}
	key := objectKey(name, ns, intLabels, groupVersionKind)
	if obj, ok := cache.objects[key]; ok {
		switch obj.(type) {
		case unstructured.Unstructured:
			unstrObj := obj.(unstructured.Unstructured)
			return &unstrObj, nil
		case toumbstone:
			ts := obj.(toumbstone)
			return nil, &ts.err
		default:
			err = fmt.Errorf("wrong type of cached object %T", obj)
			logf.Error(err, "")
			return nil, &ComposableError{err, false}
		}
	}
	var unstrObj unstructured.Unstructured
	if nameOK {
		unstrObj.SetGroupVersionKind(groupVersionKind)
		var objNamespacedname types.NamespacedName
		if res.Namespaced {
			objNamespacedname = types.NamespacedName{Namespace: ns, Name: name}
		} else {
			objNamespacedname = types.NamespacedName{Name: name}
		}
		logf.V(1).Info("Get input object", "obj", objNamespacedname, "groupVersionKind", groupVersionKind)
		err := r.Get(context.TODO(), objNamespacedname, &unstrObj)
		if err != nil {
			logf.Info("Get object returned ", "err", err, "obj", objNamespacedname)
			compErr = &ComposableError{err, true}
			cache.objects[key] = toumbstone{err: *compErr}
			return nil, compErr
		}
	} else { //labelsOK
		strLabels := make(map[string]string)
		for key, value := range intLabels {
			strValue := fmt.Sprintf("%v", value)
			strLabels[key] = strValue
		}
		unstrList := unstructured.UnstructuredList{}
		unstrList.SetGroupVersionKind(groupVersionKind)
		err = r.List(context.TODO(), &unstrList, client.InNamespace(ns), client.MatchingLabels(strLabels))
		if err != nil {
			logf.Info("list object returned ", "err", err, "namespace", ns, "labels", strLabels, "groupVersionKind", groupVersionKind)
			compErr = &ComposableError{err, true}
			cache.objects[key] = toumbstone{err: *compErr}
			return nil, compErr
		}
		itms := len(unstrList.Items)
		if itms == 1 {
			unstrObj = unstrList.Items[0]
			cache.objects[key] = unstrObj
		} else {
			err = fmt.Errorf("list object returned %d items ", itms)
			logf.Error(err, "wrong # of items", "items", itms, "namespace", Namespace, "labels", strLabels, "groupVersionKind", groupVersionKind)
			compErr = &ComposableError{err, true}
			cache.objects[key] = toumbstone{err: *compErr}
			return nil, compErr
		}
	}
	return &unstrObj, nil
}

func resolveValue2(val map[string]interface{}, unstrObj unstructured.Unstructured, path string) (interface{}, *ComposableError) {
	j := jsonpath.New("compose")
	// add ".Object" to the path
	path = path[:1] + objectPrefix + path[1:]
	err := j.Parse(path)
	if err != nil {
		logf.Error(err, "jsonpath.Parse", "path", path)
		return nil, &ComposableError{err, false}
	}
	j.AllowMissingKeys(false)

	fullResults, err := j.FindResults(unstrObj)
	if err != nil {
		logf.Error(err, "FindResults", "obj", unstrObj, "path", path)
		if strings.Contains(err.Error(), "is not found") {
			return errorToDefaultValue(val, ComposableError{err, true})
		}
		return nil, &ComposableError{err, false}
	}
	iface, ok := template.PrintableValue(fullResults[0][0])
	if !ok {
		err = fmt.Errorf("can't find printable value %v ", fullResults[0][0])
		logf.Error(err, "template.PrintableValue", "obj", unstrObj, "path", path)
		return nil, &ComposableError{err, false}
	}

	var retVal interface{}
	if transformers, ok := val[Transformers].([]interface{}); ok && len(transformers) > 0 {
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

func errorToDefaultValue(val map[string]interface{}, err ComposableError) (interface{}, *ComposableError) {
	if defaultValue, ok := val[defaultValue]; ok {
		return defaultValue, nil
	}
	return nil, &err
}

func getDiscoveryClient(cfg *rest.Config) discovery.CachedDiscoveryInterface {
	return memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(cfg))
}

func objectKey(name string, namespace string, labels map[string]interface{}, gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%s/%s/%v/%s", name, namespace, labels, gvk.String())
}

// GetNamespace gets the namespace out of a map
func GetNamespace(obj map[string]interface{}) (string, error) {
	metadata := obj[Metadata].(map[string]interface{})
	if namespace, ok := metadata[Namespace]; ok {
		return namespace.(string), nil
	}
	return "", fmt.Errorf("Failed: Template does not contain namespace")
}
