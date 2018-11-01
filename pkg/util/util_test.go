// Copyright 2019 GM Cruise LLC
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

package util

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.starlark.net/starlark"

	util "github.com/cruise-automation/isopod/pkg/testing"
)

const (
	uuidRegex = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"
)

func TestBase64(t *testing.T) {
	m := NewBase64Module()
	pkgs := starlark.StringDict{"base64": m}

	data := "data to be base64'ed"
	v, _, err := util.Eval("base64", fmt.Sprintf(`base64.encode("%s")`, data), nil, pkgs)
	if err != nil {
		t.Fatal(err)
	}

	want := "ZGF0YSB0byBiZSBiYXNlNjQnZWQ="
	got := string(v.(starlark.String))
	if want != got {
		t.Fatalf("%v: Unexpected return value.\nWant:%q\nGot: %q", m, want, got)
	}

	v, _, err = util.Eval("base64", fmt.Sprintf("base64.decode('%s')", got), nil, pkgs)
	if err != nil {
		t.Fatalf("%v: Unexpected expr error: %v", m, err)
	}

	want = data
	got = string(v.(starlark.String))
	if want != got {
		t.Errorf("%v: Unexpected return value.\nWant:%q\nGot: %q", m, want, got)
	}
}

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

func TestHTTP(t *testing.T) {
	for _, tc := range []struct {
		name      string
		expr      string
		retBody   string
		retStatus int

		wantErrMsg  string
		wantReqData string
		wantRespVal starlark.Value
		wantHeaders map[string][]string
	}{
		{
			name:        "GET success",
			expr:        `http.get(test_url, headers={"Accept-Encoding": ["gzip", "bzip"], "foo": "bar"})`,
			retBody:     "foobar",
			wantRespVal: starlark.String("foobar"),
			wantHeaders: map[string][]string{
				"Accept-Encoding": {"gzip", "bzip"},
				"Foo":             {"bar"},
			},
		},
		{
			name:        "POST success",
			expr:        `http.post(test_url, data="foobar")`,
			wantReqData: "foobar",
		},
		{
			name:        "POST success",
			expr:        `http.post(test_url, data="foobar")`,
			wantReqData: "foobar",
		},
		{
			name:        "PUT success",
			expr:        `http.put(test_url, data="foobar")`,
			wantReqData: "foobar",
		},
		{
			name:        "PATCH success",
			expr:        `http.patch(test_url, data="foobar")`,
			wantReqData: "foobar",
		},
		{
			name: "DELETE success",
			expr: "http.delete(test_url)",
		},
		{
			name:       "GET non-200 status",
			expr:       "http.get(test_url)",
			retStatus:  http.StatusTeapot,
			wantErrMsg: "418 I'm a teapot",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotHeaders map[string][]string
			var gotReqData string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k := range tc.wantHeaders {
					if gotHeaders == nil {
						gotHeaders = make(map[string][]string)
					}
					gotHeaders[k] = r.Header[k]
				}

				bs, err := ioutil.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("Failed to read request body: %v", err)
					http.Error(w, "failed to read request body", http.StatusInternalServerError)
					return
				}
				gotReqData = string(bs)

				if tc.retStatus != 0 {
					w.WriteHeader(tc.retStatus)
					return
				}

				fmt.Fprint(w, tc.retBody)
			}))
			defer ts.Close()

			pkgs := starlark.StringDict{
				"http":     NewHTTPModule(),
				"test_url": starlark.String(ts.URL),
			}

			gotVal, _, gotErr := util.Eval("http", tc.expr, nil, pkgs)

			var gotErrMsg string
			if gotErr != nil {
				gotErrMsg = gotErr.(*starlark.EvalError).Msg
			}

			if d := cmp.Diff(tc.wantErrMsg, gotErrMsg); d != "" {
				t.Fatalf("Unexpected error. (-want +got)\n%s", d)
			}
			if tc.wantErrMsg != "" { // Short-circuit for errors.
				return
			}

			if d := cmp.Diff(tc.wantReqData, gotReqData); d != "" {
				t.Errorf("Unexpected request body: (-want +got)\n%s", d)
			}

			if tc.wantRespVal == nil {
				tc.wantRespVal = starlark.None
			}
			if d := cmp.Diff(tc.wantRespVal, gotVal); d != "" {
				t.Errorf("Unexpected expression return value: (-want +got)\n%s", d)
			}

			if d := cmp.Diff(tc.wantHeaders, gotHeaders); d != "" {
				t.Errorf("Unexpected headers: (-want +got)\n%s", d)
			}
		})
	}
}

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
