// Copyright 2020 GM Cruise LLC
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

package dep

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"go.starlark.net/starlark"

	"github.com/cruise-automation/isopod/pkg/addon"
	"github.com/cruise-automation/isopod/pkg/loader"
)

const (
	// DepsFile is the file name that stores remote module info.
	DepsFile = "isopod.deps"
)

var (
	// asserts *AbstractDependency implements starlark.HasAttrs interface.
	_ starlark.HasAttrs = (*AbstractDependency)(nil)

	// Workspace is the directory that stages all Isopod-managed remote modules.
	Workspace = "/tmp/isopod-workspace"
)

// AbstractDependency contains the common impl of all loader.Dependency.
// Specifically, it offers easy parsing of
//     dependency_directive(foo="bar", baz="qux")
type AbstractDependency struct {
	*addon.SkyCtx
	typeStr string
}

// NewAbstractDependency creates a new AbstractDependency.
func NewAbstractDependency(
	typeStr string,
	requiredFields []string,
	kwargs []starlark.Tuple,
) (*AbstractDependency, error) {
	required := map[string]struct{}{}
	for _, field := range requiredFields {
		required[field] = struct{}{}
	}
	absDep := &AbstractDependency{
		SkyCtx:  addon.NewCtx(),
		typeStr: typeStr,
	}
	for _, kwarg := range kwargs {
		k := string(kwarg[0].(starlark.String))
		v := kwarg[1]
		delete(required, k)
		if err := absDep.SetField(k, v); err != nil {
			return nil, fmt.Errorf("<%s> cannot process field `%v=%v`", typeStr, k, v)
		}
	}
	for unsetKey := range required {
		return nil, fmt.Errorf("<%s> requires field `%s'", typeStr, unsetKey)
	}
	return absDep, nil
}

// String implements starlark.Value.String.
func (a *AbstractDependency) String() string {
	return fmt.Sprintf("<%s: %v>", a.Type(), a.SkyCtx.Attrs)
}

// Type implements starlark.Value.Type.
func (a *AbstractDependency) Type() string { return a.typeStr }

// Load processes the file that stores Isopod dependencies and registers them
// with the module loader to support subsequent load() statements.
func Load(entryfile string) error {
	pkgs := starlark.StringDict{
		"git_repository": NewGitRepoBuiltin(),
	}
	thread := &starlark.Thread{
		Load: loader.NewModulesLoaderWithPredeclaredPkgs(filepath.Dir(entryfile), pkgs).Load,
	}

	absPath, err := filepath.Abs(entryfile)
	if err != nil {
		return err
	}
	bytes, err := ioutil.ReadFile(absPath)
	if err != nil {
		return err
	}

	_, err = starlark.ExecFile(thread, entryfile, bytes, pkgs)
	return err
}
