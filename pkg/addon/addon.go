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

// Package addon implements "addon" built-in and its life-cycle hooks.
package addon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	log "github.com/golang/glog"
	"go.starlark.net/starlark"

	"github.com/cruise-automation/isopod/pkg/loader"
)

// Addon implements single addons lifecycle hooks.
type Addon struct {
	// TODO(dmitry.ilyevskiy): Place these inside subclassed context.
	Name     string
	filepath string
	baseDir  string
	ctx      starlark.StringDict

	// List of globally scopped symbols from main addon file exeution.
	globals starlark.StringDict

	// Predeclared packages.
	pkgs   starlark.StringDict
	loader loader.ModulesLoader

	// Defines "print" built-in function.
	printFn func(t *starlark.Thread, s string)
}

// NewAddonBuiltin returns new *starlark.Builtin for Addon with pre-declared
// pkgs.
func NewAddonBuiltin(baseDir string, pkgs starlark.StringDict) *starlark.Builtin {
	return starlark.NewBuiltin(
		"addon",
		func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name, path string
			var ctxVal starlark.Value
			if err := starlark.UnpackPositionalArgs(b.Name(), args, nil, 2, &name, &path, &ctxVal); err != nil {
				return nil, err
			}

			ctx := starlark.StringDict{}
			if ctxVal != nil {
				switch aCtx := ctxVal.(type) {
				case *SkyCtx:
					ctx = aCtx.Attrs
				case *starlark.Dict:
					for _, kv := range aCtx.Items() {
						k, v := kv[0], kv[1]
						s, ok := k.(starlark.String)
						if !ok {
							return nil, fmt.Errorf("%v context key not a string (got a %s)", k, k.Type())
						}
						ctx[string(s)] = v
					}
				default:
					return nil, fmt.Errorf("unexpected context object %v (want either ctx or starlark dict), got: %v", ctxVal, ctxVal.Type())
				}
			}

			return &Addon{
				Name:     name,
				filepath: path,
				baseDir:  baseDir,
				loader:   loader.NewModulesLoaderWithPredeclaredPkgs(baseDir, pkgs),
				ctx:      ctx,
				pkgs:     pkgs,
				globals:  starlark.StringDict{},
				printFn: func(t *starlark.Thread, msg string) {
					fmt.Fprintf(os.Stderr, "%s: %s\n", t.CallStack().At(0).Pos, msg)
				},
			}, nil
		})
}

// NewAddonForTest returns an *Addon for testing.
func NewAddonForTest(name, filepath string, ctx, pkgs starlark.StringDict, f loader.ModuleReaderFactory, printW io.Writer) *Addon {
	return &Addon{
		Name:     name,
		filepath: filepath,
		ctx:      ctx,
		pkgs:     pkgs,
		globals:  starlark.StringDict{},
		printFn: func(_ *starlark.Thread, msg string) {
			if _, err := printW.Write([]byte(msg)); err != nil {
				log.Errorf("failed to write `%s' to printFn writer: %v", msg, err)
			}
		},
		loader: loader.NewFakeModulesLoader(pkgs, f),
	}
}

func (a *Addon) StringPretty() string { return fmt.Sprintf("%s (%s)", a.Name, a.filepath) }

// String implements starlark.Value.String.
func (a *Addon) String() string { return fmt.Sprintf("<addon: %s>", a.Name) }

// Type implements starlark.Value.Type.
func (a *Addon) Type() string { return "addon" }

// Freeze implements starlark.Value.Freeze.
func (a *Addon) Freeze() {}

// Truth implements starlark.Value.Truth.
func (a *Addon) Truth() starlark.Bool { return starlark.True }

// Hash implements starlark.Value.Hash.
func (a *Addon) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", a.Type()) }

// Load loads addon from its source and executes it.
func (a *Addon) Load(ctx context.Context) (err error) {
	a.globals, err = a.loader.Load(nil, a.filepath)
	return
}

// LoadedModules returns a mapping of loaded module paths to their text context.
func (a *Addon) LoadedModules() map[string]string {
	return a.loader.GetLoaded()
}

// GetModule returns the version of loaded module
func (a *Addon) GetModule() *loader.Module {
	return a.loader.GetLoadedModule(a.filepath)
}

// Match is an optional matching hook. Returns true if addon matched the
// context and wishes to be installed.
func (a *Addon) Match(ctx context.Context) (bool, error) {
	return false, errors.New("`match' is not implemented")
}

// Status returns current addon status.
// TODO(dmitry.ilyevskiy): Make return value structured.
func (a *Addon) Status(ctx context.Context) (string, error) {
	return "", errors.New("`status' is not implemented")
}

const (
	// SkyCtxKey is a key of a thread-local value for a *SkyCtx object that
	// is set for addon execution (accessible by built-ins).
	SkyCtxKey = "context"
	// GoCtxKey is same as SkyCtxKey but for context.Context passed from
	// main runtime.
	GoCtxKey = "go_context"
)

// Install is called to install an addon.
// Callback defined by the plugin must perform all necessary work to install
// the plugin.
//
// Available built-ins:
//  * TODO(dmitry.ilyevskiy): `kube' - controls Kubernets deployments.
//  * TODO(dmitry.ilyevskiy): `gcloud' - access to GCP API.
//  * TODO(dmitry.ilyevskiy): `vault' - access to Vault.
//  * TODO(dmitry.ilyevskiy): `url' - Generic HTTP client.
func (a *Addon) Install(ctx context.Context) error {
	sCtx := &SkyCtx{Attrs: a.ctx}
	thread := &starlark.Thread{
		Print: a.printFn,
		Load:  a.loader.Load,
	}
	if a.GetModule().Version() != "" {
		sCtx.Attrs["addon_version"] = starlark.String(a.GetModule().Version())
	}

	thread.SetLocal(GoCtxKey, ctx)
	thread.SetLocal(SkyCtxKey, sCtx)

	fn, ok := a.globals["install"]
	if !ok {
		return fmt.Errorf("no `install' function found in %q", a.filepath)
	}
	if _, ok = fn.(starlark.Callable); !ok {
		return fmt.Errorf("%s must be a function (got a %s)", fn, fn.Type())
	}

	log.Infof("Running `install' for [%s] with context: %v", a.Name, a.ctx)

	args := starlark.Tuple([]starlark.Value{sCtx})
	_, err := starlark.Call(thread, fn, args, nil)
	return err
}

// Remove is called to remove the addon.
// Executes `remove' addon callback. Returns error if it doesn't exist (or
// if the callback returns error).
// TODO(dmitry.ilyevskiy): context must contain opaque info returned by install.
func (a *Addon) Remove(ctx context.Context) error {
	sCtx := &SkyCtx{Attrs: a.ctx}
	thread := &starlark.Thread{
		Print: a.printFn,
	}
	thread.SetLocal(GoCtxKey, ctx)
	thread.SetLocal(SkyCtxKey, sCtx)

	fn, ok := a.globals["remove"]
	if !ok {
		return fmt.Errorf("no `remove' function found in %q", a.filepath)
	}
	if _, ok = fn.(starlark.Callable); !ok {
		return fmt.Errorf("%s must be a function (got a %s)", fn, fn.Type())
	}

	log.Infof("Running `remove' for [%s] with context: %v", a.Name, a.ctx)

	args := starlark.Tuple([]starlark.Value{sCtx})
	_, err := starlark.Call(thread, fn, args, nil)
	return err
}

// ErrorFn implements built-in for interrupting addon execution flow on error
// and printing its message.
func ErrorFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &msg); err != nil {
		return nil, err
	}

	return starlark.None, fmt.Errorf("<%v>: %s\n\n%s", b.Name(), msg, t.CallStack())
}

// SleepFn implements built-in for sleep.
func SleepFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var dur string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &dur); err != nil {
		return nil, err
	}

	d, err := time.ParseDuration(dur)
	if err != nil {
		return nil, fmt.Errorf("<%v>: can not parse duration string `%s': %v", b.Name(), dur, err)
	}

	time.Sleep(d)

	return starlark.None, nil
}
