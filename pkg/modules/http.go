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

package modules

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"go.starlark.net/starlark"

	isopod "github.com/cruise-automation/isopod/pkg"
	"github.com/cruise-automation/isopod/pkg/addon"
)

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
