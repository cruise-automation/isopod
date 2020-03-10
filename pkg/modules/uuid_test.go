package modules

import (
	"errors"
	"fmt"
	"regexp"
	"testing"

	"go.starlark.net/starlark"

	util "github.com/cruise-automation/isopod/pkg/testing"
)

const (
	uuidRegex = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"
)

func TestUUID(t *testing.T) {
	m := NewUUIDModule()
	pkgs := starlark.StringDict{"uuid": m}
	data := "paas-dev"
	for _, tc := range []struct {
		name, expression string
		deterministic    bool
	}{
		{
			name:          "v3",
			expression:    fmt.Sprintf(`uuid.v3("%s")`, data),
			deterministic: true,
		},
		{
			name:          "v4",
			expression:    "uuid.v4()",
			deterministic: false,
		},
		{
			name:          "v5",
			expression:    fmt.Sprintf(`uuid.v5("%s")`, data),
			deterministic: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uuidgen := func() string {
				v, _, err := util.Eval("uuid", tc.expression, nil, pkgs)
				if err != nil {
					t.Fatal(err)
				}
				got := string(v.(starlark.String))
				match, err := regexp.MatchString(uuidRegex, got)
				if err != nil {
					t.Errorf("error during regexp.MatchString: %v", err)
				}
				if !match {
					t.Errorf("Result does not match UUID regex")
				}
				return got
			}
			first := uuidgen()
			second := uuidgen()
			if (first == second) != tc.deterministic {
				t.Errorf("Expect determinism is %v but was %v", tc.deterministic, (first == second))
			}
		})
	}
}

func TestUUIDErrorCase(t *testing.T) {
	m := NewUUIDModule()
	pkgs := starlark.StringDict{"uuid": m}
	for _, tc := range []struct {
		name, expression string
		expectedErr      error
	}{
		{
			name:        "v3 needs exactly one argument",
			expression:  fmt.Sprintf(`uuid.v3()`),
			expectedErr: errors.New("uuid.v3: got 0 arguments, want 1"),
		},
		{
			name:        "v5 needs exactly one argument",
			expression:  fmt.Sprintf(`uuid.v5()`),
			expectedErr: errors.New("uuid.v5: got 0 arguments, want 1"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := util.Eval("uuid", tc.expression, nil, pkgs)
			if err.Error() != tc.expectedErr.Error() {
				t.Errorf("Want error: %v, got: %v", tc.expectedErr, err)
			}
		})
	}
}
