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
