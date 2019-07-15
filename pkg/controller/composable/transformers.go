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
	"strings"
)

const (
	Base642String 	= "Base642String"
	String2Base64 	= "String2Base64"
	//Int2String 		= "int2String"
	String2Int		= "String2Int"
	//Float2String	= "float2String"
	String2Float	= "String2Float"
	Array2CSString  = "Array2CSString"
	ToString  		= "ToString"
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
	case ToString:
		return ToStringTransformer, nil
	case Base642String:
			return Base642StringTransformer, nil
	case String2Base64:
			return String2Base64Transformer, nil
	//case Int2String:
	//	return Int2StringTransformer, nil
	case String2Int:
		return String2IntTransformer, nil
	//case Float2String:
	//	return Float2StringTransformer, nil
	case String2Float:
		return String2FloatTransformer, nil
	case Array2CSString:
		return Array2CSStringTransformer, nil
	default:
		return nil, fmt.Errorf("Wrong transformer name %v", transformerName)
	}

}

func Array2CSStringTransformer (value interface{}) (interface{}, error) {
	var str strings.Builder
	if strArray, ok := value.([]string); ok {
		for i, v := range strArray {
			str.WriteString(v)
			if i != len(strArray)-1 {
				str.WriteString(",")
			}
		}
		return str.String(), nil
	} else if intArray, ok := value.([]interface{}); ok {
		for i, v := range intArray {
			if strVal, ok := v.(string); ok {
				str.WriteString(strVal)
				if i != len(intArray)-1 {
					str.WriteString(",")
				}
			}
		}
		return str.String(), nil
	}
	return nil, fmt.Errorf("The given %v has type %T, and it is not a string арраы", value, value)
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

func ToStringTransformer (value interface{}) (interface{}, error) {
	return fmt.Sprintf("%v", value), nil
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

