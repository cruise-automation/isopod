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
	"testing"

	"go.starlark.net/starlark"

	util "github.com/cruise-automation/isopod/pkg/testing"
)

func TestVault(t *testing.T) {
	tv, closeFn, err := NewFake()
	defer closeFn()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		desc string
		expr string

		wantResult string
		wantErr    string
	}{
		{
			desc:       "Write value to `foo/bar'",
			expr:       "vault.write('foo/bar', a='1', b='2')",
			wantResult: "None",
		},
		{
			desc:       "Write list value to `foo/bar2'",
			expr:       "vault.write('foo/bar2', a=['1','2'], b='2')",
			wantResult: "None",
		},
		{
			desc:       "Read raw data from `foo/bar'",
			expr:       "vault.read_raw('foo/bar')",
			wantResult: `map["data":map["a":"1" "b":"2"]]`,
		},
		{
			desc:       "Check if `foo/bar' exists",
			expr:       "vault.exist('foo/bar')",
			wantResult: "True",
		},
		{
			desc:       "Read data from `foo/bar'",
			expr:       "vault.read('foo/bar')",
			wantResult: `map["a":"1" "b":"2"]`,
		},
		{
			desc:       "Read data from `foo/bar2'",
			expr:       "vault.read('foo/bar2')",
			wantResult: `map["a":["1", "2"] "b":"2"]`,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
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
