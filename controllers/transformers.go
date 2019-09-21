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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	Base64ToString  = "Base64ToString"
	StringToBase64  = "StringToBase64"
	StringToInt     = "StringToInt"
	StringToFloat   = "StringToFloat"
	StringToBool    = "StringToBool"
	ArrayToCSString = "ArrayToCSString"
	JsonToObject    = "JsonToObject"
	ObjectToJson    = "ObjectToJson"
	ToString        = "ToString"
)

// Base Transformer function
type Transformer func(interface{}) (interface{}, error)

// Compound Transformer function
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

// Compound Transformer function
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
	case JsonToObject:
		return JsonToObjectTransformer, nil
	case ObjectToJson:
		return ObjectToJsonTransformer, nil
	default:
		return nil, fmt.Errorf("Wrong transformer name %q", transformerName)
	}

}

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

func JsonToObjectTransformer(value interface{}) (interface{}, error) {
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

func ObjectToJsonTransformer(value interface{}) (interface{}, error) {
	retVal, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(retVal), err
}

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

func String2Base64Transformer(value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		return base64.StdEncoding.EncodeToString([]byte(strValue)), nil
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

func ToStringTransformer(value interface{}) (interface{}, error) {
	return fmt.Sprintf("%v", value), nil
}

func String2IntTransformer(value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		if n, err := strconv.Atoi(strValue); err == nil {
			return n, nil
		} else {
			return nil, err
		}
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

func String2FloatTransformer(value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		if f, err := strconv.ParseFloat(strValue, 64); err == nil {
			return f, nil
		} else {
			return nil, err
		}
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

func String2BoolTransformer(value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		if f, err := strconv.ParseBool(strValue); err == nil {
			return f, nil
		} else {
			return nil, err
		}
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}
