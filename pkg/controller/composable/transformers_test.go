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
	"testing"
)

type Planet struct {
	Name string
	YearSpan int
}
func TestArray2CSStringTransformer(t *testing.T) {

	var tests = []struct {
		value   interface{}
		exp     string
	}{
		{[]string{"a", "b", "cd", "efg"}, "a,b,cd,efg"},
		{[]int32{1, 2, 34, 567}, "1,2,34,567"},
		{[]int64{1, 2, 34, 567}, "1,2,34,567"},
		{[]float32{1.1, 2.2, 34.3, 567.0, 1.234560e+02}, "1.1,2.2,34.3,567,123.456"},
		{[]float64{1.1, 2.2, 34.3, 567.0, 1.234560e+02}, "1.1,2.2,34.3,567,123.456"},
		{[]bool{true, false, true}, "true,false,true"},
		{[]Planet{ {Name: "Mercury", YearSpan: 88 }, {Name: "Venus", YearSpan: 243}, {Name: "Earth", YearSpan: 365 }},
			"{Mercury 88},{Venus 243},{Earth 365}"},
		{"test", "test"},
		{12, "12"},
		{true, "true"},

	}

	for _, e := range tests {
		t.Logf("inputValue = %v\n", e.value)
		retValue, err := Array2CSStringTransformer(e.value)
		if err != nil {
			t.Fatalf("An unexpected error occurred: %v", err)
		}
		t.Logf("retValue = %v\n", retValue)
		if strRetValue, ok := retValue.(string); ok {
			if strRetValue == e.exp {
				continue
			}
			t.Fatalf("retruned str %q is not equal to expected string %q",strRetValue, e.exp )
		}
		t.Fatalf("retruned value is not string [%T]", retValue)
	}
}

func TestCompoundTransformerNames(t *testing.T) {
	var tests = []struct {
		value   			interface{}
		transformerNames 	[]string
		exp     			interface{}
	}{
		{12, []string{ToString, StringToInt}, 12},
		{"12", []string{StringToInt, ToString}, "12"},
		{12.2, []string{ToString, StringToFloat}, 12.2},
		{"12.2", []string{StringToFloat, ToString}, "12.2"},
		{true, []string{ToString, StringToBool}, true},
		{"true", []string{StringToBool, ToString}, "true"},
		{13, []string{ToString, StringToBase64, Base64ToString, StringToInt}, 13},
		{true, []string{ToString, StringToBase64, Base64ToString, StringToBool}, true},
	}
	for _, e := range tests {
		t.Logf("inputValue = %v, transformers %v\n", e.value, e.transformerNames)
		retValue, err := CompoundTransformerNames(e.value, e.transformerNames...)
		if err != nil {
			t.Fatalf("An unexpected error occurred: %v", err)
		}
		t.Logf("retValue = %v\n", retValue)
		if retValue != e.exp {
			t.Fatalf("retruned value [%v] is not equal to expected one [%v]",retValue, e.exp )
		}
	}

}