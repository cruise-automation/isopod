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
	"go.starlark.net/starlark"
)

// Predeclared returns a starlark.StringDict containing predeclared modules
// from util:
//   * base64 - Base64 encode/decode operations (RFC 4648).
//   * uuid - UUID generate operations (RFC 4122).
//   * http - HTTP calls.
//   * struct - Starlark struct with to_json() support.
func Predeclared() starlark.StringDict {
	return starlark.StringDict{
		"base64": NewBase64Module(),
		"uuid":   NewUUIDModule(),
		"http":   NewHTTPModule(),
		"struct": starlark.NewBuiltin("struct", StructFn),
	}
}
