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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

type Planet struct {
	Name     string
	YearSpan int
}

var _ = Describe("./pkg/controller/composable/trnsformers", func() {
	DescribeTable("Test CompoundTransformerNames",
		func(inputVal interface{}, transformerNames []string, expectedVal interface{}) {
			Ω(CompoundTransformerNames(inputVal, transformerNames...)).Should(Equal(expectedVal))
		},
		Entry("ToString, StringToInt", 12, []string{ToString, StringToInt}, 12),
		Entry("StringToInt, ToString", "12", []string{StringToInt, ToString}, "12"),
		Entry("ToString, StringToFloat", 12.2, []string{ToString, StringToFloat}, 12.2),
		Entry("StringToFloat, ToString", "12.2", []string{StringToFloat, ToString}, "12.2"),
		Entry("StringToFloat, ToString", true, []string{ToString, StringToBool}, true),
		Entry("StringToBool, ToString", "true", []string{StringToBool, ToString}, "true"),
		Entry("ToString, StringToBase64, Base64ToString, StringToInt", 13, []string{ToString, StringToBase64, Base64ToString, StringToInt}, 13),
		Entry("ToString, StringToBase64, Base64ToString, StringToBool", true, []string{ToString, StringToBase64, Base64ToString, StringToBool}, true),
		Entry("JsonToObject, ArrayToCSString", "[\"kafka04-prod02.messagehub.services.us-south.bluemix.net:9093\",\"kafka03-prod02.messagehub.services.us-south.bluemix.net:9093\",\"kafka05-prod02.messagehub.services.us-south.bluemix.net:9093\",\"kafka02-prod02.messagehub.services.us-south.bluemix.net:9093\",\"kafka01-prod02.messagehub.services.us-south.bluemix.net:9093\"]",
			[]string{JSONToObject, ArrayToCSString},
			"kafka04-prod02.messagehub.services.us-south.bluemix.net:9093,kafka03-prod02.messagehub.services.us-south.bluemix.net:9093,kafka05-prod02.messagehub.services.us-south.bluemix.net:9093,kafka02-prod02.messagehub.services.us-south.bluemix.net:9093,kafka01-prod02.messagehub.services.us-south.bluemix.net:9093"),
		Entry("Base64ToString, JsonToObject, ArrayToCSString", "WyJrYWZrYTA0LXByb2QwMi5tZXNzYWdlaHViLnNlcnZpY2VzLnVzLXNvdXRoLmJsdWVtaXgubmV0OjkwOTMiLCJrYWZrYTAzLXByb2QwMi5tZXNzYWdlaHViLnNlcnZpY2VzLnVzLXNvdXRoLmJsdWVtaXgubmV0OjkwOTMiLCJrYWZrYTA1LXByb2QwMi5tZXNzYWdlaHViLnNlcnZpY2VzLnVzLXNvdXRoLmJsdWVtaXgubmV0OjkwOTMiLCJrYWZrYTAyLXByb2QwMi5tZXNzYWdlaHViLnNlcnZpY2VzLnVzLXNvdXRoLmJsdWVtaXgubmV0OjkwOTMiLCJrYWZrYTAxLXByb2QwMi5tZXNzYWdlaHViLnNlcnZpY2VzLnVzLXNvdXRoLmJsdWVtaXgubmV0OjkwOTMiXQo=",
			[]string{Base64ToString, JSONToObject, ArrayToCSString},
			"kafka04-prod02.messagehub.services.us-south.bluemix.net:9093,kafka03-prod02.messagehub.services.us-south.bluemix.net:9093,kafka05-prod02.messagehub.services.us-south.bluemix.net:9093,kafka02-prod02.messagehub.services.us-south.bluemix.net:9093,kafka01-prod02.messagehub.services.us-south.bluemix.net:9093"),
		Entry("ObjectToJson", []Planet{{Name: "Mercury", YearSpan: 88}, {Name: "Venus", YearSpan: 243}, {Name: "Earth", YearSpan: 365}},
			[]string{ObjectToJSON},
			"[{\"Name\":\"Mercury\",\"YearSpan\":88},{\"Name\":\"Venus\",\"YearSpan\":243},{\"Name\":\"Earth\",\"YearSpan\":365}]"))

	DescribeTable("Test Array2CSStringTransformer",
		func(inputVal interface{}, expectedVal interface{}) {
			Ω(Array2CSStringTransformer(inputVal)).Should(Equal(expectedVal))
		},
		Entry("strings array", []string{"a", "b", "cd", "efg"}, "a,b,cd,efg"),
		Entry("int32 array", []int32{1, 2, 34, 567}, "1,2,34,567"),
		Entry("int64 array", []int64{1, 2, 34, 567}, "1,2,34,567"),
		Entry("float32 array", []float32{1.1, 2.2, 34.3, 567.0, 1.234560e+02}, "1.1,2.2,34.3,567,123.456"),
		Entry("float64 array", []float64{1.1, 2.2, 34.3, 567.0, 1.234560e+02}, "1.1,2.2,34.3,567,123.456"),
		Entry("boolean array", []bool{true, false, true}, "true,false,true"),
		Entry("objects array", []Planet{{Name: "Mercury", YearSpan: 88}, {Name: "Venus", YearSpan: 243}, {Name: "Earth", YearSpan: 365}},
			"{Mercury 88},{Venus 243},{Earth 365}"),
		Entry("single string", "test", "test"),
		Entry("single int", 12, "12"),
		Entry("single boolean", true, "true"))
})
