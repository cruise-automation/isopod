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

package vault

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.starlark.net/starlark"

	util "github.com/cruise-automation/isopod/pkg/testing"
)

// fakeServer creates fake Vault endpoint.
func fakeServer(t *testing.T, wantValues map[string]string, rawData string) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/foo/bar" {
			http.Error(w, "not found", http.StatusNotFound)
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		switch r.Method {
		case http.MethodGet:
			m := json.RawMessage(rawData)
			b, err := m.MarshalJSON()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				t.Error(err)
				return
			}

			if _, err := w.Write(b); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				t.Error(err)
				return
			}
		case http.MethodPut:
			bs, err := json.Marshal(wantValues)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				t.Error(err)
				return
			}
			wantValues := string(bs)

			bs, err = ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				t.Error(err)
				return
			}
			gotValues := string(bs)

			if d := cmp.Diff(wantValues, gotValues); d != "" {
				http.Error(w, "unexpected values", http.StatusBadRequest)
				t.Errorf("Unexpected values written: (-want +got)\n%s", d)
			}
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			t.Errorf("Unexpected method: %v", r.Method)
		}
	}))
}

func TestVault(t *testing.T) {

	for _, tc := range []struct {
		desc    string
		expr    string
		rawData string
		dryRun  bool

		wantResult string
		wantValues map[string]string
		wantErr    string
	}{
		{
			desc:       "Read secret from `foo/bar'",
			expr:       "vault.read('foo/bar')",
			rawData:    `{"data": {"a": "b", "c": 1, "d": false, "e": [1, 2, 3], "f": null}}`,
			wantResult: `map["a":"b" "c":1 "d":False "e":[1, 2, 3] "f":None]`,
		},
		{
			desc:       "Read raw data from `foo/bar'",
			expr:       "vault.read_raw('foo/bar')",
			rawData:    `{"a": {"b": "c"}}`,
			wantResult: `map["a":map["b":"c"]]`,
		},
		{
			desc:       "Write value to `foo/bar'",
			expr:       "vault.write('foo/bar', a='1', b='2')",
			wantResult: "None",
			wantValues: map[string]string{"a": "1", "b": "2"},
		},
		{
			desc:       "Dry run write",
			expr:       "vault.write('foo/bar', a='1', b='2')",
			dryRun:     true,
			wantResult: "None",
			wantValues: map[string]string{},
		},
		{
			desc:       "Check if `foo/bar' exists",
			expr:       "vault.exist('foo/bar')",
			wantResult: "True",
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ts := fakeServer(t, tc.wantValues, tc.rawData)
			defer ts.Close()

			tv, err := NewFakeWithServer(ts, tc.dryRun)
			if err != nil {
				t.Fatal(err)
			}

			pkgs := starlark.StringDict{"vault": tv}
			v, _, err := util.Eval(t.Name(), tc.expr, nil, pkgs)

			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if tc.wantErr != gotErr {
				t.Fatalf("Unexpected error.\nWant: %s\nGot: %s", tc.wantErr, gotErr)
			}

			if tc.wantResult != v.String() {
				t.Fatalf("Unexpected expression result.\nWant: %s\nGot: %s", tc.wantResult, v.String())
			}

		})
	}
}
