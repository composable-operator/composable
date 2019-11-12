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

package v1

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	// Base64ToString - name of the base64 to string transformer
	Base64ToString = "Base64ToString"

	// StringToBase64 - name of the string to base64 transformer
	StringToBase64 = "StringToBase64"

	// StringToInt - name of the string to integer transformer
	StringToInt = "StringToInt"

	// StringToFloat - name of the string to float transformer
	StringToFloat = "StringToFloat"

	// StringToBool - name of the string to boolean transformer
	StringToBool = "StringToBool"

	// ArrayToCSString - name of the array to Comma Separated String (CSS) transformer
	ArrayToCSString = "ArrayToCSString"

	// JSONToObject - name of the JSON to object transformer
	JSONToObject = "JsonToObject"

	// ObjectToJSON - name of the object to JSON transformer
	ObjectToJSON = "ObjectToJson"

	// ToString - name of the transformer, that transforms any object to its native string representation
	ToString = "ToString"
)

// Transformer - the base transformer function
type Transformer func(interface{}) (interface{}, error)

// CompoundTransformer - the function that combines several transformers
func CompoundTransformer(value interface{}, transformers ...Transformer) (interface{}, error) {
	tempValue := value
	var err error
	for _, tr := range transformers {
		tempValue, err = tr(tempValue)
		if err != nil {
			return nil, err
		}
	}
	return tempValue, nil
}

// CompoundTransformerNames returns names of the given transformers
func CompoundTransformerNames(value interface{}, transNames ...string) (interface{}, error) {
	tempValue := value
	for _, trName := range transNames {
		tr, err := string2Transformer(trName)
		if err != nil {
			return nil, err
		}
		tempValue, err = tr(tempValue)
		if err != nil {
			return nil, err
		}
	}
	return tempValue, nil
}

func string2Transformer(transformerName string) (Transformer, error) {
	switch transformerName {
	case ToString:
		return ToStringTransformer, nil
	case Base64ToString:
		return Base642StringTransformer, nil
	case StringToBase64:
		return String2Base64Transformer, nil
	case StringToInt:
		return String2IntTransformer, nil
	case StringToFloat:
		return String2FloatTransformer, nil
	case StringToBool:
		return String2BoolTransformer, nil
	case ArrayToCSString:
		return Array2CSStringTransformer, nil
	case JSONToObject:
		return JSONToObjectTransformer, nil
	case ObjectToJSON:
		return ObjectToJSONTransformer, nil
	default:
		return nil, fmt.Errorf("Wrong transformer name %q", transformerName)
	}

}

//Array2CSStringTransformer ...
func Array2CSStringTransformer(intValue interface{}) (interface{}, error) {
	var str strings.Builder
	switch reflect.TypeOf(intValue).Kind() {
	case reflect.Slice, reflect.Array:
		s := reflect.ValueOf(intValue)
		for i := 0; i < s.Len(); i++ {
			str.WriteString(fmt.Sprintf("%v", s.Index(i)))
			if i != s.Len()-1 {
				str.WriteString(",")
			}
		}
		return str.String(), nil
	default:
		return fmt.Sprintf("%v", intValue), nil
	}
}

// JSONToObjectTransformer ...
func JSONToObjectTransformer(value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		var data interface{}
		err := json.Unmarshal([]byte(strValue), &data)
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a JSON string", value, value)
}

// ObjectToJSONTransformer ...
func ObjectToJSONTransformer(value interface{}) (interface{}, error) {
	retVal, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(retVal), err
}

// Base642StringTransformer ...
func Base642StringTransformer(value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		decoded, err := base64.StdEncoding.DecodeString(strValue)
		if err != nil {
			return nil, err
		}
		return string(decoded), nil
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

// String2Base64Transformer ...
func String2Base64Transformer(value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		return base64.StdEncoding.EncodeToString([]byte(strValue)), nil
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

// ToStringTransformer ...
func ToStringTransformer(value interface{}) (interface{}, error) {
	return fmt.Sprintf("%v", value), nil
}

// String2IntTransformer ...
func String2IntTransformer(value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		n, err := strconv.Atoi(strValue)
		if err == nil {
			return n, nil
		}
		return nil, err
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

// String2FloatTransformer ...
func String2FloatTransformer(value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		f, err := strconv.ParseFloat(strValue, 64)
		if err == nil {
			return f, nil
		}
		return nil, err
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

// String2BoolTransformer ...
func String2BoolTransformer(value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		f, err := strconv.ParseBool(strValue)
		if err == nil {
			return f, nil
		}
		return nil, err
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}
