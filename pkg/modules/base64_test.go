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
