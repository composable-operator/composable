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
	"encoding/base64"
	"fmt"
	"strconv"
)

const (
	Base642String 	= "base642String"
	String2Base64 	= "string2Base64"
	Int2String 		= "int2String"
	String2Int		= "string2Int"
	Float2String	= "float2String"
	String2Float	= "string2Float"
)

// Base Transformer function
type Transformer func(interface{}) (interface{}, error)

// Compound Transformer function
func CompoundTransformer (value interface{}, transformers ...Transformer) (interface{}, error) {
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
func CompoundTransformerNames (value interface{}, transNames ...string) (interface{}, error) {
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
	case Base642String:
			return Base642StringTransformer, nil
	case String2Base64:
			return String2Base64Transformer, nil
	case Int2String:
		return Int2StringTransformer, nil
	case String2Int:
		return String2IntTransformer, nil
	case Float2String:
		return Float2StringTransformer, nil
	case String2Float:
		return String2FloatTransformer, nil
	default:
		return nil, fmt.Errorf("Wrong transformer name %v", transformerName)
	}

}

func Base642StringTransformer (value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		decoded, err := base64.StdEncoding.DecodeString(strValue)
		if err != nil {
			return nil, err
		}
		return string(decoded), nil
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

func String2Base64Transformer (value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		return base64.StdEncoding.EncodeToString([]byte(strValue)), nil
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

func Int2StringTransformer (value interface{}) (interface{}, error) {
	if intValue, ok := value.(int); ok {
		return strconv.Itoa(intValue), nil
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not an integer", value, value)
}

func String2IntTransformer (value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		if n, err := strconv.Atoi(strValue); err == nil {
			return n, nil
		} else {
			return nil, err
		}
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

func Float2StringTransformer (value interface{}) (interface{}, error) {
	if floatValue, ok := value.(float64); ok {
		return fmt.Sprintf("%f", floatValue), nil
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a float", value, value)
}

func String2FloatTransformer (value interface{}) (interface{}, error) {
	if strValue, ok := value.(string); ok {
		if f, err := strconv.ParseFloat(strValue, 64); err == nil {
			return f, nil
		} else {
			return nil, err
		}
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string", value, value)
}

