package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/third_party/forked/golang/template"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var logf = log.Log.WithName("composable-sdk")

// String constants
const (
	Metadata       = "metadata"
	Namespace      = "namespace"
	GetValueFrom   = "getValueFrom"
	kind           = "kind"
	apiVersion     = "apiVersion"
	path           = "path"
	Name           = "name"
	Labels         = "labels"
	Transformers   = "format-transformers"
	defaultValue   = "defaultValue"
	objectPrefix   = ".Object"
	kindNotFound   = "Error resolving the kind for an object reference"
	objectNotFound = "Error finding an object reference"
	valueNotFound  = "Error finding a value in an object reference"
	illFormedRef   = "Object reference is ill-formed"
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

	result, comperr := resolve(ctx, k.Client, k.ResourcesClient, objectMap, namespace)
	if comperr != nil {
		return comperr
	}

	inrec, err = json.Marshal(result)
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
// This method assumes that the objMap is an object that has a metadata section with a namespace defined
func resolve(ctx context.Context, r client.Client, discoveryClient discovery.ServerResourcesInterface, objMap map[string]interface{}, defaultNamespace string) (interface{}, error) {
	obj, err := resolveFields(ctx, r, objMap, defaultNamespace, discoveryClient)
	if err != nil {
		return nil, err
	}
	// ret := unstructured.Unstructured{Object: obj.(map[string]interface{})}
	return obj, nil
}

func resolveFields(ctx context.Context, r client.Client, fields interface{}, composableNamespace string, discoveryClient discovery.ServerResourcesInterface) (interface{}, error) {
	switch fields.(type) {
	case map[string]interface{}:
		if fieldsOut, ok := fields.(map[string]interface{}); ok {
			for k, v := range fieldsOut {
				var newFields interface{}
				var err error
				if k == GetValueFrom {
					newFields, err = resolveValue(ctx, r, v, composableNamespace, discoveryClient)
					if err != nil {
						logf.Info("resolveFields resolveValue 1", "err", err)
						return nil, err
					}
					fields = newFields
				} else if values, ok := v.(map[string]interface{}); ok {
					if value, ok := values[GetValueFrom]; ok {
						if len(values) > 1 {
							err := fmt.Errorf("%s, %s", "GetValueFrom must be the only field in a value", illFormedRef)
							logf.Error(err, "resolveFields", "values", values)
							return nil, err
						}
						newFields, err = resolveValue(ctx, r, value, composableNamespace, discoveryClient)
					} else {
						newFields, err = resolveFields(ctx, r, values, composableNamespace, discoveryClient)
					}
					if err != nil {
						logf.Info("resolveFields resolveValue 2", "err", err)
						return nil, err
					}
					fieldsOut[k] = newFields
				} else if values, ok := v.([]interface{}); ok {
					for i, value := range values {
						newFields, err := resolveFields(ctx, r, value, composableNamespace, discoveryClient)
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
				newFields, err := resolveFields(ctx, r, value, composableNamespace, discoveryClient)
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

func lookupAPIResource(discoveryClient discovery.ServerResourcesInterface, objKind, apiVersion string) (*metav1.APIResource, error) {
	log.Log.V(1).Info("lookupAPIResource", "objKind", objKind, "apiVersion", apiVersion)
	var resources []*metav1.APIResourceList
	var err error
	if len(apiVersion) > 0 {
		list, err := discoveryClient.ServerResourcesForGroupVersion(apiVersion)
		if err != nil {
			logf.Error(err, "lookupAPIResource", "apiVersion", apiVersion)
			return nil, err
		}
		resources = []*metav1.APIResourceList{list}
		log.Log.V(1).Info("lookupAPIResource", "list", list, "apiVersion", apiVersion)
	} else {
		resources, err = discoveryClient.ServerPreferredResources()
		if err != nil {
			logf.Error(err, "lookupAPIResource ServerPreferredResources")
			return nil, err
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
			return nil, err
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
		return nil, err
	}

	if targetResource != nil {
		return targetResource, nil
	}
	err = fmt.Errorf("Unable to find api resource named %q ", kind)
	logf.Error(err, "lookupAPIResource")
	return nil, err
}

func resolveValue(ctx context.Context, r client.Client, value interface{}, composableNamespace string, discoveryClient discovery.ServerResourcesInterface) (interface{}, error) {
	// r.log.Info("resolveValue", "value", value)
	var err error
	if val, ok := value.(map[string]interface{}); ok {
		if objKind, ok := val[kind].(string); ok {
			apiversion := ""
			if apiversion, ok = val[apiVersion].(string); !ok {
				apiversion = ""
			}
			if path, ok := val[path].(string); ok {
				if strings.HasPrefix(path, "{.") {

					unstrObj, err := getInputObject(ctx, r, val, objKind, apiversion, composableNamespace, discoveryClient)
					if err != nil {
						if IsRefNotFound(err) {
							// we have checked the object and did not find it
							val, err1 := errorToDefaultValue(val, err)
							return val, err1
						}
						// we should not be here
						return nil, err
					}
					return resolveValue2(val, *unstrObj, path)
				}
				err = fmt.Errorf("%s, %s ", "GetValueFrom is not well-formed, 'path' is not jsonpath formated", illFormedRef)
				logf.Error(err, "resolveValue", "path", path)
				return nil, err
			}
			err = fmt.Errorf("%s, %s ", "GetValueFrom is not well-formed, 'path' is not defined", illFormedRef)
			logf.Error(err, "resolveValue", "val", val)
			return nil, err
		}
		err = fmt.Errorf("%s, %s ", "GetValueFrom is not well-formed, 'kind' is not defined ", illFormedRef)
		logf.Error(err, "resolveValue", "val", val)
		return nil, err
	}
	err = fmt.Errorf("%s %T, %s ", "GetValueFrom is not well-formed, value type is not ", value, illFormedRef)
	logf.Error(err, "resolveValue", "value", value)
	return nil, err
}

func getInputObject(ctx context.Context, r client.Client, val map[string]interface{}, objKind, apiversion, composableNamespace string, discoveryClient discovery.ServerResourcesInterface) (*unstructured.Unstructured, error) {
	res, err := lookupAPIResource(discoveryClient, objKind, apiversion)
	if err != nil {
		err := fmt.Errorf("%s, %s", err.Error(), kindNotFound)
		// We cannot resolve input object API resource, so we return error even if a default value is set.
		return nil, err
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
	name, nameOK := val[Name].(string)
	intLabels, labelsOK := val[Labels].(map[string]interface{})
	if nameOK == labelsOK { // only one of them should be defined
		if nameOK && labelsOK {
			err = fmt.Errorf("%s, %s", "GetValueFrom is not well-formed, both 'name' and 'labels' cannot be defined at the same time", illFormedRef)
		} else {
			err = fmt.Errorf("%s, %s", "GetValueFrom is not well-formed, neither 'name' nor 'labels' are defined (one expected)", illFormedRef)
		}
		logf.Error(err, "getInputObject", "val", val)
		return nil, err
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
		err := r.Get(ctx, objNamespacedname, &unstrObj)
		if err != nil {
			logf.Info("Get object returned ", "err", err, "obj", objNamespacedname)
			err = fmt.Errorf("%s, %s", err.Error(), objectNotFound)
			return nil, err
		}
	} else { // labelsOK
		strLabels := make(map[string]string)
		for key, value := range intLabels {
			strValue := fmt.Sprintf("%v", value)
			strLabels[key] = strValue
		}
		unstrList := unstructured.UnstructuredList{}
		unstrList.SetGroupVersionKind(groupVersionKind)
		err = r.List(ctx, &unstrList, client.InNamespace(ns), client.MatchingLabels(strLabels))
		if err != nil {
			logf.Info("list object returned ", "err", err, "namespace", ns, "labels", strLabels, "groupVersionKind", groupVersionKind)
			err = fmt.Errorf("%s, %s", err.Error(), objectNotFound)
			return nil, err
		}
		itms := len(unstrList.Items)
		if itms == 1 {
			unstrObj = unstrList.Items[0]
		} else {
			err = fmt.Errorf("list object returned %d items ", itms)
			logf.Error(err, "wrong # of items", "items", itms, "namespace", Namespace, "labels", strLabels, "groupVersionKind", groupVersionKind)
			err = fmt.Errorf("%s, %s", err.Error(), objectNotFound)
			return nil, err
		}
	}
	return &unstrObj, nil
}

func resolveValue2(val map[string]interface{}, unstrObj unstructured.Unstructured, path string) (interface{}, error) {
	j := jsonpath.New("compose")
	// add ".Object" to the path
	path = path[:1] + objectPrefix + path[1:]
	err := j.Parse(path)
	if err != nil {
		logf.Error(err, "jsonpath.Parse", "path", path)
		return nil, err
	}
	j.AllowMissingKeys(false)

	fullResults, err := j.FindResults(unstrObj)
	if err != nil {
		logf.Error(err, "FindResults", "obj", unstrObj, "path", path)
		if strings.Contains(err.Error(), "is not found") {
			err = fmt.Errorf("%s, %s", err.Error(), valueNotFound)
			return errorToDefaultValue(val, err)
		}
		return nil, err
	}
	iface, ok := template.PrintableValue(fullResults[0][0])
	if !ok {
		err = fmt.Errorf("%s %v, %s", "can't find printable value ", fullResults[0][0], valueNotFound)
		logf.Error(err, "template.PrintableValue", "obj", unstrObj, "path", path)
		return nil, err
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

func errorToDefaultValue(val map[string]interface{}, err error) (interface{}, error) {
	if defaultValue, ok := val[defaultValue]; ok {
		return defaultValue, nil
	}
	return nil, err
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
	return "", fmt.Errorf("Failed: Object does not contain namespace")
}

// IsRefNotFound can be used to determine if an error returned by the ResolvedObject method is due to the reference not being found
func IsRefNotFound(err error) bool {
	return IsKindNotFound(err) || IsObjectNotFound(err) || IsValueNotFound(err)
}

// IsKindNotFound can be used to determine if an error returned by the ResolveObject method is kindNotFound
func IsKindNotFound(err error) bool {
	return strings.Contains(err.Error(), kindNotFound)
}

// IsObjectNotFound can be used to determine if an error returned by the ResolveObject method is objectNotFound
func IsObjectNotFound(err error) bool {
	return strings.Contains(err.Error(), objectNotFound)
}

// IsValueNotFound can be used to determine if an error returned by the ResolveObject method is valueNotFound
func IsValueNotFound(err error) bool {
	return strings.Contains(err.Error(), valueNotFound)
}

// IsIllFormedRef can be used to determine if an error returned by the ResolveObject method is illFormedRef
func IsIllFormedRef(err error) bool {
	return strings.Contains(err.Error(), illFormedRef)
}
