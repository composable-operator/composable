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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	composablev1alpha1 "github.com/ibm/composable/api/v1alpha1"
	sdk "github.com/ibm/composable/sdk"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	getValueFrom = sdk.GetValueFrom
)

// log is for logging validation activities.
var validatelog = logf.Log.WithName("controllers").WithName("Composable-validate")

func validateComposable(comp *composablev1alpha1.Composable) error {
	validatelog.Info("Enter validation", "resourcename", comp.Name)
	if comp.Spec.Template == nil {
		err := fmt.Errorf("Missing spec.template")
		validatelog.Info("Finish validation", "error", err)
		return err
	}
	allErrs := validateAPIVersionKind(comp.Name, comp.Spec.Template, field.NewPath("spec").Child("template"))
	err := validate(comp.Name, comp.Spec.Template, field.NewPath("spec").Child("template"))
	if err != nil {
		allErrs = append(allErrs, err...)
	}
	if len(allErrs) > 0 {
		validatelog.Info("Finish validation", "errors", allErrs)
		return apierrors.NewInvalid(schema.GroupKind{Group: "ibmcloud.ibm.com", Kind: "Composable"}, comp.Name, allErrs)
	}

	validatelog.Info("Finish validation with no errors")
	return nil
}

// validateAPIVersionKind validates the template content for required fields of apiVersion and kind
func validateAPIVersionKind(name string, template *runtime.RawExtension, fieldpath *field.Path) field.ErrorList {
	var f interface{}
	json.Unmarshal(template.Raw, &f)
	m := f.(map[string]interface{})
	var allErrs field.ErrorList

	if m["apiVersion"] == nil || len(m["apiVersion"].(string)) == 0 {
		allErrs = append(allErrs, field.Required(fieldpath.Child("apiVersion"), "missing required field \"apiVersion\""))
	}
	if m["kind"] == nil || len(m["kind"].(string)) == 0 {
		validatelog.Info("validateApiVersionKind - kind is empty")
		allErrs = append(allErrs, field.Required(fieldpath.Child("kind"), "missing required field \"kind\""))
	}
	return allErrs
}

// validate validates the required fields of getValueFrom elements in the template
func validate(name string, template *runtime.RawExtension, fieldpath *field.Path) field.ErrorList {
	var f interface{}
	json.Unmarshal(template.Raw, &f)
	m := f.(map[string]interface{})
	err := findGetValueFrom(name, fieldpath, m)
	return err
}

// findGetValueFrom parses the template content recursively for getValueFrom elements and validates them
func findGetValueFrom(name string, path *field.Path, m map[string]interface{}) field.ErrorList {
	var allErrs field.ErrorList
	for k, v := range m {
		mykey := path.Child(k)
		switch vv := v.(type) {
		case string, int32, int64, float32, float64, bool:
		case []interface{}:
			if err := findGetValueFrom2(name, mykey, vv); err != nil {
				allErrs = append(allErrs, err...)
			}
		case map[string]interface{}:
			if vv[getValueFrom] != nil {
				if err := validateGetValueFrom(vv[getValueFrom], mykey.Child(getValueFrom)); err != nil {
					allErrs = append(allErrs, err...)
				}
			} else { //recursive checking the sub-elements
				if err := findGetValueFrom(name, mykey, vv); err != nil {
					allErrs = append(allErrs, err...)
				}
			}
		default:
			validatelog.Info("findGetValueFrom", "key", mykey, "type-unknown", vv)
		}
	}
	return allErrs
}

// findGetValueFrom2 functions in the same way as findGetValueFrom above except taking []interface{} as inputs
// findGetValueFrom2 processes the ararys in the template content for getValueFrom elements
func findGetValueFrom2(name string, path *field.Path, m []interface{}) field.ErrorList {
	var allErrs field.ErrorList
	for k, v := range m {
		mykey := path.Child(strconv.Itoa(k))
		switch vv := v.(type) {
		case string, int32, int64, float32, float64, bool:
		case []interface{}:
			if err := findGetValueFrom2(name, mykey, vv); err != nil {
				allErrs = append(allErrs, err...)
			}
		case map[string]interface{}:
			if vv[getValueFrom] != nil {
				if err := validateGetValueFrom(vv[getValueFrom], mykey.Child(getValueFrom)); err != nil {
					allErrs = append(allErrs, err...)
				}
			} else {
				if err := findGetValueFrom(name, mykey, vv); err != nil {
					allErrs = append(allErrs, err...)
				}
			}
		default:
			validatelog.Info("findGetValueFrom", "key", k, "type-unknown", vv)
		}
	}
	return allErrs
}

// validateGetValueFrom validates the syntax of input getValueFrom fields
func validateGetValueFrom(v interface{}, key *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	getValueFrom := sdk.ComposableGetValueFrom{}
	var labelsExist bool = false

	var labelsValue string
	if val, ok := v.(map[string]interface{}); ok {
		if labels, ok := val["labels"].(map[string]interface{}); ok {
			labelsExist = ok
			labelsValue = fmt.Sprintf("%v", labels)
			labelsValue = strings.Replace(labelsValue, "map[", "", -1)
			labelsValue = strings.Replace(labelsValue, "]", "", -1)
		}
	}

	obj, err := json.Marshal(v)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(key, v, err.Error()))
		return allErrs
	}
	json.Unmarshal(obj, &getValueFrom)

	if len(getValueFrom.Name) == 0 && !labelsExist {
		allErrs = append(allErrs, field.Required(key.Child("name"), "missing required field \"name\" or \"labels\""))
		allErrs = append(allErrs, field.Required(key.Child("labels"), "missing required field \"name\" or \"labels\""))
	} else if len(getValueFrom.Name) > 0 && labelsExist {
		allErrs = append(allErrs, field.Invalid(key.Child("name"), getValueFrom.Name, "cannot specify both \"name\" and \"labels\" (only one of them can be specified)"))
		allErrs = append(allErrs, field.Invalid(key.Child("labels"), labelsValue, "cannot specify both \"name\" and \"labels\" (only one of them can be specified)"))
	}
	if len(getValueFrom.Kind) == 0 {
		allErrs = append(allErrs, field.Required(key.Child("kind"), "missing required field \"kind\""))
	}
	if len(getValueFrom.Path) == 0 {
		allErrs = append(allErrs, field.Required(key.Child("path"), "missing required field \"path\""))
	}
	//validatelog.Info("validateGetValueFrom", "transformers size", len(getValueFrom.FormatTransformers))
	if len(getValueFrom.FormatTransformers) > 0 {
		errs := validateTransformers(getValueFrom.FormatTransformers, key.Child("format-transformers"))
		if errs != nil {
			allErrs = append(allErrs, errs...)
		}
	}
	return allErrs
}

func validateTransformers(names []string, key *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	var isValid bool
	transformers := [10]string{sdk.Base64ToString,
		sdk.StringToBase64,
		sdk.StringToInt,
		sdk.StringToInt32,
		sdk.StringToFloat,
		sdk.ArrayToCSString,
		sdk.StringToBool,
		sdk.ToString,
		sdk.JSONToObject,
		sdk.ObjectToJSON,
	}

	for j := 0; j < len(names); j++ {
		isValid = false
		for i := 0; i < len(transformers); i++ {
			if names[j] == transformers[i] {
				isValid = true
				break
			}
		}
		if !isValid {
			allErrs = append(allErrs, field.Invalid(key, names[j], "unknown format-transformer"))
		}
	}
	return allErrs
}
