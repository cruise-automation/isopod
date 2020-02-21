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

// package vault implements "vault" built-in package to access Vault API from
// an addon.
package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	vault "github.com/hashicorp/vault/api"
	"go.starlark.net/starlark"

	isopod "github.com/cruise-automation/isopod/pkg"
	"github.com/cruise-automation/isopod/pkg/addon"
	"github.com/cruise-automation/isopod/pkg/util"
)

// vaultPackage implements Vault API package.
type vaultPackage struct {
	*isopod.Module
	client *vault.Client
}

// New returns a new skaylark.HasAttrs object for vault package.
func New(c *vault.Client) *isopod.Module {
	v := &vaultPackage{
		client: c,
	}
	v.Module = &isopod.Module{
		Name: "vault",
		Attrs: starlark.StringDict{
			"read":     starlark.NewBuiltin("vault.read", v.vaultReadFn),
			"read_raw": starlark.NewBuiltin("vault.read_raw", v.vaultReadRawFn),
			"write":    starlark.NewBuiltin("vault.write", v.vaultWriteFn),
			"exist":    starlark.NewBuiltin("vault.exist", v.vaultExistFn),
		},
	}
	return v.Module
}

// vaultReadFn is a starlark built-in function that reads a secret value from
// vault.
// Returns a (potentially nested) dict of secret data by the specified Vault
// path.
// Usage:
//   values = vault.read(path)
//   print(values['foo'])
func (p *vaultPackage) vaultReadFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &path); err != nil {
		return nil, fmt.Errorf("<%v>: failed to parse args: %v", b.Name(), err)
	}

	r := p.client.NewRequest("GET", "/v1/"+path)

	ctx := t.Local(addon.GoCtxKey).(context.Context)
	resp, err := p.client.RawRequestWithContext(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), err)
	}
	if err := resp.Error(); err != nil {
		return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), err)
	}

	s, err := vault.ParseSecret(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("<%v>: failed to parse secret data: %v", b.Name(), err)
	}
	if s == nil { // vault client is dumb.
		return starlark.None, nil
	}

	v, err := util.ValueFromNestedMap(s.Data)
	if err != nil {
		return nil, fmt.Errorf("<%v>: failed to parse data: %v", b.Name(), err)
	}
	return v, nil
}

// vaultReadRawFn is a starlark built-in function that reads a raw JSON value
// from vault endpoint.
// Returns a (potentially nested) dict of raw JSON data read by the specified
// Vault endpoint path.
// Usage:
//   values = vault.read_raw(path)
//   print(values['foo'])
func (p *vaultPackage) vaultReadRawFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &path); err != nil {
		return nil, fmt.Errorf("<%v>: failed to parse args: %v", b.Name(), err)
	}

	r := p.client.NewRequest("GET", "/v1/"+path)

	ctx := t.Local(addon.GoCtxKey).(context.Context)
	resp, err := p.client.RawRequestWithContext(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), err)
	}
	if err := resp.Error(); err != nil {
		return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), err)
	}

	d := json.NewDecoder(resp.Body)
	data := map[string]interface{}{}
	if err := d.Decode(&data); err != nil {
		return nil, fmt.Errorf("<%v>: failed to decode raw JSON data: %v", b.Name(), err)
	}

	v, err := util.ValueFromNestedMap(data)
	if err != nil {
		return nil, fmt.Errorf("<%v>: failed to parse data: %v", b.Name(), err)
	}
	return v, nil
}

// vaultWriteFn is a starlark built-in function that writes to Vault.
// Usage:
//   # kwargs keyword names are used as data keys, values are stored as repr
//   # of a kwarg value.
//   vault.write(path, key1=value1, key2=value2)
//   data = vault.read(path)
//   print(data['key1']) == repr(value1) # Must be True
func (p *vaultPackage) vaultWriteFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, nil, 1, &path); err != nil {
		return nil, fmt.Errorf("<%v>: failed to parse args: %v", b.Name(), err)
	}

	data := make(map[string]interface{}, len(kwargs))
	for _, kv := range kwargs {
		switch value := kv[1].(type) {
		case starlark.String:
			data[string(kv[0].(starlark.String))] = string(value)
		case *starlark.List:
			list := make([]string, value.Len())
			for i := 0; i < value.Len(); i++ {
				ss, ok := value.Index(i).(starlark.String)
				if !ok {
					return nil, fmt.Errorf("<%v>: list value not a string: %v", b.Name(), value)
				}
				list[i] = string(ss)
			}
			data[string(kv[0].(starlark.String))] = list
		default:
			return nil, fmt.Errorf("<%v>: value not a string or list: %v", b.Name(), kv[1])
		}
	}

	r := p.client.NewRequest("PUT", "/v1/"+path)
	if err := r.SetJSONBody(data); err != nil {
		return nil, fmt.Errorf("failed to set request body to %+v: %v", data, err)
	}

	ctx := t.Local(addon.GoCtxKey).(context.Context)
	resp, err := p.client.RawRequestWithContext(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), err)
	}
	if err := resp.Error(); err != nil {
		return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), err)
	}

	d := json.NewDecoder(resp.Body)
	respData := map[string]interface{}{}
	if err := d.Decode(&respData); err != nil {
		return starlark.None, nil
	}

	v, err := util.ValueFromNestedMap(respData)
	if err != nil {
		return starlark.None, nil
	}
	return v, nil
}

// vaultExistFn is a starlark built-in function that checks if a secret path exists on vault.
//
// Checking the vault response status seems to be the most resilient implementation. The alternative
// would be to list all secrets under filepath.Dir(path) to match filepath.Base(path), but then
// filepath.Dir(path) itself could be nonexistent, causing isopod to exit.
//
// Usage:
//   ok = vault.exist(path)
//	 if ok:
//	 	print(path + " exists on vault.")
func (p *vaultPackage) vaultExistFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &path); err != nil {
		return nil, fmt.Errorf("<%v>: failed to parse args: %v", b.Name(), err)
	}
	r := p.client.NewRequest("GET", "/v1/"+path)

	ctx := t.Local(addon.GoCtxKey).(context.Context)
	resp, err := p.client.RawRequestWithContext(ctx, r)
	if resp.StatusCode == http.StatusNotFound {
		return starlark.False, nil
	}
	if err != nil {
		return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), err)
	}
	if err := resp.Error(); err != nil {
		return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), err)
	}

	return starlark.True, nil
}
