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
