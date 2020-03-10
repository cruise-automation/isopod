package modules

import (
	"github.com/google/uuid"

	"go.starlark.net/starlark"

	isopod "github.com/cruise-automation/isopod/pkg"
)

var (
	seedUUID = uuid.MustParse("00000000-0000-0000-0000-000000000000")
)

// NewUUIDModule returns a uuid module.
func NewUUIDModule() *isopod.Module {
	return &isopod.Module{
		Name: "uuid",
		Attrs: map[string]starlark.Value{
			"v3": starlark.NewBuiltin("uuid.v3", uuidGenerateV3Fn),
			"v4": starlark.NewBuiltin("uuid.v4", uuidGenerateV4Fn),
			"v5": starlark.NewBuiltin("uuid.v5", uuidGenerateV5Fn),
		},
	}
}

// uuidGenerateV3Fn is a built-in to generate type 3 UUID digest from input data.
func uuidGenerateV3Fn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, nil, 1, &v); err != nil {
		return nil, err
	}

	result := uuid.NewMD5(seedUUID, []byte(v))
	return starlark.String(result.String()), nil
}

// uuidGenerateV4Fn is a built-in to generate type 4 UUID.
func uuidGenerateV4Fn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(uuid.New().String()), nil
}

// uuidGenerateV3Fn is a built-in to generate type 5 UUID digest from input data.
func uuidGenerateV5Fn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, nil, 1, &v); err != nil {
		return nil, err
	}

	result := uuid.NewSHA1(seedUUID, []byte(v))
	return starlark.String(result.String()), nil
}
