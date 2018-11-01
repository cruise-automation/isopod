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

package util

import (
	"bytes"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Struct adds to_json attribute to the starlark struct.
type Struct struct {
	*starlarkstruct.Struct
}

// StructFn implements the built-in function that instantiates and extends the
// starlark struct to support to_json().
func StructFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	s, err := starlarkstruct.Make(t, b, args, kwargs)
	if err != nil {
		return nil, err
	}
	return &Struct{
		Struct: s.(*starlarkstruct.Struct),
	}, nil
}

// Attr implements starlark.HasAttrs.Attr.
func (s *Struct) Attr(name string) (starlark.Value, error) {
	if name == "to_json" {
		return starlark.NewBuiltin(name, func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var buf bytes.Buffer
			err := WriteJSON(&buf, s.Struct)
			if err != nil {
				return nil, err
			}
			return starlark.String(buf.String()), nil
		}), nil
	}
	return s.Struct.Attr(name)
}
