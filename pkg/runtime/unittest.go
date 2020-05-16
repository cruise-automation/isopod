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

package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stripe/skycfg"
	"go.starlark.net/starlark"

	isopod "github.com/cruise-automation/isopod/pkg"
	"github.com/cruise-automation/isopod/pkg/addon"
	"github.com/cruise-automation/isopod/pkg/cloud/gke"
	"github.com/cruise-automation/isopod/pkg/cloud/onprem"
	"github.com/cruise-automation/isopod/pkg/kube"
	"github.com/cruise-automation/isopod/pkg/loader"
	"github.com/cruise-automation/isopod/pkg/modules"
	"github.com/cruise-automation/isopod/pkg/vault"
)

func isTest(name string) bool {
	return strings.HasSuffix(name, "_test.ipd")
}

// Search looks in the path for test files.
// Will walk all subdirs if path provided with /... suffix, list all dir files
// if path is a directory and match a single file otherwise.
// Matches by _test.ipd suffix.
// If path is empty will search from current path recursively.
func search(path string) ([]string, error) {
	if path == "" {
		path = "./..."
	}

	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	var out []string
	if strings.HasSuffix(path, "/...") {
		err := filepath.Walk(filepath.Dir(path), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() && isTest(info.Name()) {
				out = append(out, path)
			}

			return nil
		})
		if err != nil {
			return nil, err
		}
		return out, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if info.IsDir() { // All files immediately under this directory.
		fs, err := f.Readdir(0)
		if err != nil {
			return nil, err
		}

		for _, f := range fs {
			if !f.IsDir() && isTest(f.Name()) {
				out = append(out, filepath.Join(path, f.Name()))
			}
		}
	} else if isTest(info.Name()) {
		out = []string{filepath.Join(path, info.Name())}
	}

	return out, nil
}

type assertErr struct {
	err error
}

func (e *assertErr) Error() string {
	return e.err.Error()
}

func makeAssertFn() *starlark.Builtin {
	return starlark.NewBuiltin(
		"assert",
		func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var cond bool
			var msg string
			if err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &cond, &msg); err != nil {
				return nil, err
			}

			if !cond {
				res := fmt.Sprintf("%v: assertion failed", thread.CallStack().At(0).Pos)
				if msg != "" {
					res += fmt.Sprintf(": %s", msg)
				}
				return nil, &assertErr{errors.New(res)}
			}

			return starlark.None, nil
		})
}

// result records test status, output and telemetry.
type result struct {
	Pass       bool
	Path       string
	FailureMsg string
	Output     io.Reader
	Runtime    time.Duration
}

// exec executes all test cases within a file referenced by path.
func exec(ctx context.Context, path string) (*result, error) {
	v, vClose, err := vault.NewFake()
	if err != nil {
		return nil, err
	}
	defer vClose()

	k, kClose, err := kube.NewFake(false)
	if err != nil {
		return nil, err
	}
	defer kClose()

	pkgs := starlark.StringDict{
		"assert": makeAssertFn(),
		"vault":  v,
		"kube":   k,
		"gke":    gke.NewGKEBuiltin("sa-kay-not-used-since-mocked", "Isopod"),
		"onprem": onprem.NewOnPremBuiltin("fake-kubeconfig"),
		"error":  starlark.NewBuiltin("error", addon.ErrorFn),
		"sleep":  starlark.NewBuiltin("sleep", addon.SleepFn),
	}

	scPkgs := skycfg.UnstablePredeclaredModules(&protoRegistry{})
	for name, pkg := range scPkgs {
		pkgs[name] = pkg
	}

	// Must be loaded last to ensure our impl of struct() persists.
	for k, v := range modules.Predeclared() {
		pkgs[k] = v
	}

	startT := time.Now()

	out := new(bytes.Buffer)
	outFn := func(_ *starlark.Thread, msg string) { fmt.Println(msg) }
	thread := &starlark.Thread{
		Print: outFn,
		Load:  loader.NewModulesLoaderWithPredeclaredPkgs(filepath.Dir(path), pkgs).Load,
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	globals, err := starlark.ExecFile(thread, path, data, pkgs)
	if err != nil {
		return nil, err
	}

	for name, v := range globals {
		if !strings.HasPrefix(name, "test_") {
			continue
		}
		fn, ok := v.(starlark.Callable)
		if !ok {
			return nil, fmt.Errorf("%s must be a function (got a %s)", v, v.Type())
		}

		sCtx := addon.NewCtx()

		thread := &starlark.Thread{
			Print: outFn,
		}
		thread.SetLocal(addon.GoCtxKey, ctx)
		thread.SetLocal(addon.SkyCtxKey, sCtx)

		tCtx := &isopod.Module{
			Name: "test_ctx",
			Attrs: starlark.StringDict{
				"ctx": sCtx,
			},
		}
		args := starlark.Tuple([]starlark.Value{tCtx})

		_, err := starlark.Call(thread, fn, args, nil)
		if err != nil {
			if aErr, ok := err.(*assertErr); ok {
				return &result{
					Pass:       false,
					Path:       path,
					FailureMsg: aErr.Error(),
					Output:     out,
					Runtime:    time.Since(startT),
				}, nil
			}

			return nil, err
		}
	}

	return &result{
		Pass:    true,
		Path:    path,
		Output:  out,
		Runtime: time.Since(startT),
	}, nil
}

// RunUnitTests executes (if found) tests reference by path. Writes test
// output to w.
func RunUnitTests(ctx context.Context, path string, outW, errW io.Writer) (bool, error) {
	ts, err := search(path)
	if err != nil {
		return false, err
	}
	if len(ts) == 0 {
		fmt.Fprintf(outW, "No tests found.\n")
		return true, nil
	}

	var rs []*result
	for _, t := range ts {
		res, err := exec(ctx, t)
		if err != nil {
			fmt.Fprintf(errW, "%v\n", err)
			rs = append(rs, &result{
				Pass: false,
				Path: t,
			})
			continue
		}
		rs = append(rs, res)
	}

	status := true
	for _, r := range rs {
		if !r.Pass {
			if r.FailureMsg != "" {
				fmt.Fprintf(outW, "FAIL: %s\n", r.FailureMsg)
			}
			fmt.Fprintf(outW, "FAIL\t%s\n", r.Path)
			status = false
		} else {
			fmt.Fprintf(outW, "ok\t%s %v\n", r.Path, r.Runtime)
		}
	}

	return status, nil
}
