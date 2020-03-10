package modules

import (
	"encoding/base64"

	"go.starlark.net/starlark"

	isopod "github.com/cruise-automation/isopod/pkg"
)

// NewBase64Module returns a base64 module.
func NewBase64Module() *isopod.Module {
	return &isopod.Module{
		Name: "base64",
		Attrs: map[string]starlark.Value{
			"encode": starlark.NewBuiltin("base64.encode", base64EncodeFn),
			"decode": starlark.NewBuiltin("base64.decode", base64DecodeFn),
		},
	}
}

// base64EncodeFn is a built-in to encode string arg in base64.
func base64EncodeFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, nil, 1, &v); err != nil {
		return nil, err
	}

	return starlark.String(base64.StdEncoding.EncodeToString([]byte(v))), nil
}

// base64DecodeFn is a built-in that decodes string from base64.
func base64DecodeFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, nil, 1, &v); err != nil {
		return nil, err
	}

	data, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, err
	}

	return starlark.String(string(data)), nil
}
