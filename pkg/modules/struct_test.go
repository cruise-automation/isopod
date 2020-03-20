// Copyright 2020 Cruise LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modules

import (
	"fmt"
	"testing"

	"go.starlark.net/starlark"

	util "github.com/cruise-automation/isopod/pkg/testing"
)

func TestStruct(t *testing.T) {
	pkgs := starlark.StringDict{"struct": starlark.NewBuiltin("struct", StructFn)}
	for _, tc := range []struct {
		name, expression, expectedValue string
	}{
		{
			name:          "Simple struct",
			expression:    fmt.Sprintf(`struct(foo="bar").to_json()`),
			expectedValue: `{"foo": "bar"}`,
		},
		{
			name:          "Nested structs",
			expression:    fmt.Sprintf(`struct(foo="bar", baz=struct(qux="bar")).to_json()`),
			expectedValue: `{"baz": {"qux": "bar"}, "foo": "bar"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			val, _, err := util.Eval("struct", tc.expression, nil, pkgs)
			if err != nil {
				t.Errorf("Expect nil err but got %v", err)
			}
			sv, ok := val.(starlark.String)
			if !ok {
				t.Error("to_json() should return starlark string")
			}
			if string(sv) != tc.expectedValue {
				t.Errorf("Want value: %v, got: %v", tc.expectedValue, string(sv))
			}
		})
	}
}
