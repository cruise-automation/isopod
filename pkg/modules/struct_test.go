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
