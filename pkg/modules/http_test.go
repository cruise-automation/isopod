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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.starlark.net/starlark"

	util "github.com/cruise-automation/isopod/pkg/testing"
)

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
