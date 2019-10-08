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

package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var composablelog = logf.Log.WithName("controllers").WithName("Composable-webhook")

const (
	// OperationCreate - name of create operation
	OperationCreate = "CREATE"
	// OperationUpdate - name of update operation
	OperationUpdate = "UPDATE"
)

// SetupWebhookWithManager sets up the webhooks with the manager
func (r *Composable) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-ibmcloud-ibm-com-v1alpha1-composable,mutating=true,failurePolicy=fail,groups=ibmcloud.ibm.com,resources=composables,verbs=create;update,versions=v1alpha1,name=mcomposable.kb.io

var _ webhook.Defaulter = &Composable{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Composable) Default() {
	composablelog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
	// Laura: not implement in this version
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-ibmcloud-ibm-com-v1alpha1-composable,mutating=false,failurePolicy=fail,groups=ibmcloud.ibm.com,resources=composables,versions=v1alpha1,name=vcomposable.kb.io

var _ webhook.Validator = &Composable{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Composable) ValidateCreate() error {
	composablelog.Info("validate create", "name", r.Name)
	return r.validateComposable(OperationCreate)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Composable) ValidateUpdate(old runtime.Object) error {
	composablelog.Info("validate update", "name", r.Name)
	return r.validateComposable(OperationUpdate)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Composable) ValidateDelete() error {
	composablelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

// validateComposable validates the spec.template of the request
func (r *Composable) validateComposable(operation string) error {
	composablelog.Info("validateComposable", "name", r.Name)
	allErrs := r.validateAPIVersionKind(r.Spec.Template, field.NewPath("spec").Child("template"))
	m, err := r.validate(r.Spec.Template, field.NewPath("spec").Child("template"))
	if err != nil {
		allErrs = append(allErrs, err...)
	}
	if len(allErrs) > 0 {
		return apierrors.NewInvalid(schema.GroupKind{Group: "ibmcloud.ibm.com", Kind: "Composable"}, r.Name, allErrs)
	}

	composablelog.Info("validateComposable", "name", r.Name, "dry-run-instance", m)
	//disable for now, need more work on inserting valid values per schema
	/*	if err := r.dryRun(m, operation); err != nil {
		allErrs = append(allErrs, err)
		return apierrors.NewInvalid(schema.GroupKind{Group: "ibmcloud.ibm.com", Kind: "Composable"}, r.Name, allErrs)
	}*/
	return nil
}

// validateAPIVersionKind validates the template content for required fields of apiVersion and kind
func (r *Composable) validateAPIVersionKind(template *runtime.RawExtension, fieldpath *field.Path) field.ErrorList {
	var f interface{}
	json.Unmarshal(template.Raw, &f)
	m := f.(map[string]interface{})
	var allErrs field.ErrorList

	if m["apiVersion"] == nil || len(m["apiVersion"].(string)) == 0 {
		allErrs = append(allErrs, field.Invalid(fieldpath.Child("apiVersion"), r.Name, "Missing required field - apiVersion"))
	}
	if m["kind"] == nil || len(m["kind"].(string)) == 0 {
		composablelog.Info("validateApiVersionKind - kind is empty")
		allErrs = append(allErrs, field.Invalid(fieldpath.Child("kind"), r.Name, "Missing required field - kind"))
	}
	return allErrs
}

// validate validates the template for the required fields of getValueFrom
func (r *Composable) validate(template *runtime.RawExtension, fieldpath *field.Path) (map[string]interface{}, field.ErrorList) {
	var f interface{}
	json.Unmarshal(template.Raw, &f)
	m := f.(map[string]interface{})
	err := r.findGetValueFrom(fieldpath, m)
	return m, err
}

// findGetValueFrom parses the template content recursively for getValueFrom elements and validates them
func (r *Composable) findGetValueFrom(path *field.Path, m map[string]interface{}) field.ErrorList {
	var allErrs field.ErrorList
	for k, v := range m {
		mykey := path.Child(k)
		switch vv := v.(type) {
		case string, int32, int64, float32, float64, bool:
		case []interface{}:
			if err := r.findGetValueFrom2(mykey, vv); err != nil {
				allErrs = append(allErrs, err...)
			}
		case map[string]interface{}:
			if vv["getValueFrom"] != nil {
				if err := validateGetValueFrom(vv["getValueFrom"]); err != nil {
					allErrs = append(allErrs, field.Invalid(mykey.Child("getValueFrom"), r.Name, err.Error()))
				}
				// TODO: set the value to an appropriate type e.g. int, string, etc
				m[k] = "abc"
			} else { //recursive checking the sub-elements
				if err := r.findGetValueFrom(mykey, vv); err != nil {
					allErrs = append(allErrs, err...)
				}
			}
		default:
			composablelog.Info("findGetValueFrom", "key", mykey, "type-unknown", vv)
		}
	}
	return allErrs
}

// findGetValueFrom2 functions in the same way as findGetValueFrom above except taking []interface{} as inputs
// findGetValueFrom2 processes the ararys in the template content for getValueFrom elements
func (r *Composable) findGetValueFrom2(path *field.Path, m []interface{}) field.ErrorList {
	var allErrs field.ErrorList
	for k, v := range m {
		mykey := path.Child(strconv.Itoa(k))
		switch vv := v.(type) {
		case string, int32, int64, float32, float64, bool:
		case []interface{}:
			if err := r.findGetValueFrom2(mykey, vv); err != nil {
				allErrs = append(allErrs, err...)
			}
		case map[string]interface{}:
			if vv["getValueFrom"] != nil {
				if err := validateGetValueFrom(vv["getValueFrom"]); err != nil {
					allErrs = append(allErrs, field.Invalid(mykey.Child("getValueFrom"), r.Name, err.Error()))
				}
				// TODO: set a random value of appropriate type for dry-run
				m[k] = "abc2"
			} else {
				if err := r.findGetValueFrom(mykey, vv); err != nil {
					allErrs = append(allErrs, err...)
				}
			}
		default:
			composablelog.Info("findGetValueFrom", "key", k, "type-unknown", vv)
		}
	}
	return allErrs
}

// validateGetValueFrom validates the syntax of input getValueFrom fields
func validateGetValueFrom(v interface{}) error {
	var missingItems []string
	getValueFrom := ComposableGetValueFrom{}

	obj, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("Invalid getValueFrom - %v", err.Error())
	}
	json.Unmarshal(obj, &getValueFrom)
	if len(getValueFrom.Name) == 0 {
		missingItems = append(missingItems, "name")
	}
	if len(getValueFrom.Kind) == 0 {
		missingItems = append(missingItems, "kind")
	}
	if len(getValueFrom.Path) == 0 {
		missingItems = append(missingItems, "path")
	}
	if len(missingItems) > 0 {
		items := array2string(missingItems)
		composablelog.Info("getValueFrom is INVALID", "missing", items)
		return fmt.Errorf("Missing required field(s) - %v", items)
	}
	return nil
}

func array2string(a []string) string {
	str := ""
	for _, v := range a {
		str = v + "," + str
	}
	str = strings.TrimRight(str, ",")
	return str
}

// dryrun as a means of syntax validation of the template content
func (r *Composable) dryRun(m map[string]interface{}, op string) *field.Error {
	cl, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("template"), r.Name, "dry-run failed to get client")
	}

	u := unstructured.Unstructured{Object: m}
	composablelog.Info("dry-run", "obj", u.Object)
	if op == OperationCreate {
		if err = cl.Create(context.TODO(), &u, client.CreateDryRunAll); err != nil {
			composablelog.Info("create dry-run failed", "name", r.Name, "err", err.Error())
			return field.Invalid(field.NewPath("spec").Child("template"), r.Name, err.Error())
		}
	}
	if op == OperationUpdate {
		if err = cl.Update(context.TODO(), &u, client.UpdateDryRunAll); err != nil {
			composablelog.Info("update dry-run failed", "name", r.Name, "err", err.Error())
			return field.Invalid(field.NewPath("spec").Child("template"), r.Name, err.Error())
		}
	}
	composablelog.Info("dry-run passed", "name", r.Name)
	return nil
}
