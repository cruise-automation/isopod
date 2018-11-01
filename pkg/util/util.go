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

// package util implements helper built-ins.
package util

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.starlark.net/starlark"

	isopod "github.com/cruise-automation/isopod/pkg"
	"github.com/cruise-automation/isopod/pkg/addon"
)

var (
	seedUUID = uuid.MustParse("00000000-0000-0000-0000-000000000000")
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

// NewHTTPModule returns new Isopod built-in module for HTTP calls.
// Supports these methods:
//  * http.get - Performs HTTP GET call
//  * http.post - Performs HTTP POST call
//  * http.put - Performs HTTP PUT call
//  * http.patch - Performs HTTP PATCH call
//  * http.delete - Performs HTTP DELETE call
//
// Args:
// url - required URL to send request to.
// headers - optional headers provided as Starlark dict. Values can be either
//           Starlark strings (for single value headers) or lists (for multiple
//           ones).
// data - optional data sent in request body (take Starlark string).
//
// Returns: Starlark string of response body. If response body is empty, returns
// starlark.None.
//
// Errors out on non-2XX response codes.
func NewHTTPModule() *isopod.Module {
	return &isopod.Module{
		Name: "http",
		Attrs: map[string]starlark.Value{
			"get":    getHTTPFn(http.MethodGet),
			"post":   getHTTPFn(http.MethodPost),
			"put":    getHTTPFn(http.MethodPut),
			"patch":  getHTTPFn(http.MethodPatch),
			"delete": getHTTPFn(http.MethodDelete),
		},
	}
}

func getHTTPFn(method string) *starlark.Builtin {
	return starlark.NewBuiltin(
		"http."+strings.ToLower(method),
		func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var url string
			hdrs := &starlark.Dict{}
			var body string
			unpacked := []interface{}{
				"url", &url,
				"headers?", &hdrs,
				"data?", &body,
			}

			if err := starlark.UnpackArgs(b.Name(), args, kwargs, unpacked...); err != nil {
				return nil, fmt.Errorf("<%v>: %v", b.Name(), err)
			}

			req, err := http.NewRequest(method, url, strings.NewReader(body))
			if err != nil {
				return nil, fmt.Errorf("failed to initialize request: %v", err)
			}

			for _, kv := range hdrs.Items() {
				k, v := kv[0], kv[1]
				sk, ok := k.(starlark.String)
				if !ok {
					return nil, fmt.Errorf("'%v header key not a string (got a %s)", k, k.Type())
				}

				switch sv := v.(type) {
				case starlark.String:
					req.Header.Add(string(sk), string(sv))
				case *starlark.List:
					iter := sv.Iterate()
					var x starlark.Value
					for iter.Next(&x) {
						sx, ok := x.(starlark.String)
						if !ok {
							return nil, fmt.Errorf("'%v` header value not a string (got a %s)", k, x.Type())
						}
						req.Header.Add(string(sk), string(sx))
					}
					iter.Done()
				default:
					return nil, fmt.Errorf("'%v` header value not a string or a list (got a %s)", k, v.Type())
				}
			}

			client := &http.Client{}
			ctx := t.Local(addon.GoCtxKey).(context.Context)
			resp, err := client.Do(req.WithContext(ctx))
			if err != nil {
				return nil, fmt.Errorf("failed to make an HTTP request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return nil, errors.New(resp.Status)
			}

			respBody, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read response body: %v", err)
			}

			// If body was empty, return None value instead of empty string.
			if len(respBody) == 0 {
				return starlark.None, nil
			}

			return starlark.String(respBody), nil
		})
}
