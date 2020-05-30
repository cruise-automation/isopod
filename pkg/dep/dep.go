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

// AbstractDependency contains the common impl of all KubernetesVendor.
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

	bytes, err := ioutil.ReadFile(entryfile)
	if err != nil {
		return err
	}

	_, err = starlark.ExecFile(thread, entryfile, bytes, pkgs)
	return err
}
