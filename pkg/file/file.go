package file

import (
	"io/ioutil"

	isopod "github.com/cruise-automation/isopod/pkg"
	"go.starlark.net/starlark"
)

type filePackage struct {
	*isopod.Module
}

// New returns a new starlark.HasAttrs object for file package.
func New() starlark.HasAttrs {
	f := &filePackage{}

	f.Module = &isopod.Module{
		Name: "file",
		Attrs: starlark.StringDict{
			"read": starlark.NewBuiltin("file.read", f.readFile),
		},
	}

	return f
}

// readFile blindly reads a file and returns it as a string
func (f *filePackage) readFile(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var filename string
	unpacked := []interface{}{
		"filename", &filename,
	}

	if err := starlark.UnpackArgs(b.Name(), args, kwargs, unpacked...); err != nil {
		return nil, err
	}
	dat, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return starlark.String(dat), nil
}
