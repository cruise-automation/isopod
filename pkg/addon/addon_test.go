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

package addon

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/cruise-automation/isopod/pkg/loader"
)

func TestAddonLoad(t *testing.T) {
	ctx := context.Background()
	bW := new(bytes.Buffer)

	testPrint := func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var msg string
		if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &msg); err != nil {
			return nil, err
		}
		bW.Write([]byte(msg))
		return starlark.None, nil
	}

	aCtx := starlark.StringDict{
		"cluster": starlark.String("test"),
	}
	pkgs := starlark.StringDict{
		"test_print": starlark.NewBuiltin("test_print", testPrint),
		"sleep":      starlark.NewBuiltin("sleep", SleepFn),
	}

	f := func(module string) (io.Reader, func(), error) {
		var text string
		switch module {
		case "module.ipd":
			text = `
def print_hello():
  test_print("hello")
`
		case "addon.ipd":
			text = `
load("module.ipd", "print_hello")

def install(ctx):
  sleep("100ms")
  print_hello()
  print("install " + ctx.cluster)
`
		}
		return strings.NewReader(text), func() {}, nil
	}

	addon := NewAddonForTest("test", "addon.ipd", aCtx, pkgs, f, bW)

	if err := addon.Load(ctx); err != nil {
		t.Fatal(err)
	}

	if err := addon.Install(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAddonInstall(t *testing.T) {
	ctx := context.Background()
	cluster := "test"

	b := new(bytes.Buffer)
	sc := bufio.NewScanner(b)

	aCtx := starlark.StringDict{
		"cluster": starlark.String(cluster),
	}
	pkgs := starlark.StringDict{
		"error": starlark.NewBuiltin("error", ErrorFn),
	}
	addon := NewAddonForTest("test", "testdata/addon.ipd", aCtx, pkgs, loader.NewFileReaderFactory("../.."), b)

	if err := addon.Load(ctx); err != nil {
		t.Fatal(err)
	}

	if err := addon.Install(ctx); err != nil {
		t.Fatal(err)
	}

	wantMsg := "install " + cluster
	if sc.Scan(); sc.Text() != wantMsg {
		t.Fatalf("Unexpected msg. Want: %q, got: %q", wantMsg, sc.Text())
	}
}
