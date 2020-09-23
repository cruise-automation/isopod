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

package vault

import (
	"context"
	"encoding/json"
	"fmt"
	isopod "github.com/cruise-automation/isopod/pkg"
	"github.com/cruise-automation/isopod/pkg/addon"
	"github.com/cruise-automation/isopod/pkg/util"
	vaultapi "github.com/hashicorp/vault/api"
	"go.starlark.net/starlark"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
)

type fakeVault struct {
	*isopod.Module
	realClient *vaultapi.Client
	m          map[string]string
}

// vaultFakeReadFn is a starlark built-in function that returns a fakeVaules Starlark dict.
// Meant for using during dry-run when we don't want vault to actually be read.
// Checks if any secret exists in the path and returns a fakeVaules Starklark dict if yes.
// Usage:
//   values = vault.read(path)
//   print(values['foo']) -> Prints "fake"
func (fvlt *fakeVault) vaultFakeReadFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if err := fvlt.assertToken(); err != nil {
		return nil, err
	}
	var path string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &path); err != nil {
		return nil, fmt.Errorf("<%v>: failed to parse args: %v", b.Name(), err)
	}

	secretName := filepath.Base("/" + path)
	parent := strings.Replace(path, "/"+secretName, "", -1)
	secretsListResp, err := fvlt.realClient.Logical().List(parent)
	if err != nil {
		return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), err)
	}
	secretsListObj, ok := secretsListResp.Data["keys"]
	if !ok {
		return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), "no keys found under this path")
	}
	secrets := secretsListObj.([]interface{})
	for _, v := range secrets {
		if secretName == v.(string) {
			return &fakeValues{}, nil
		}
	}
	return nil, fmt.Errorf("<%v>: request failed: %v", b.Name(), "requested secret was not found in this path")
}

// vaultFakeReadRawFn is starlark built-in function that reads a raw JSON value
//// from vault endpoint.
//// Returns a (potentially nested) dict of raw JSON data read by the specified
//// Vault endpoint path.
//// Usage:
////   values = vault.read_raw(path)
////   print(values['foo'])
func (fvlt *fakeVault) vaultFakeReadRawFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return fvlt.vaultFakeReadFn(t, b, args, kwargs)
}

// vaultFakeWriteFn is a starlark built-in function that writes to Vault.
//// Usage:
////   # kwargs keyword names are used as data keys, values are stored as repr
////   # of a kwarg value.
////   vault.write(path, key1=value1, key2=value2)
////   data = vault.read(path)
////   print(data['key1']) == repr(value1) # Must be True
func (fvlt *fakeVault) vaultFakeWriteFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := fvlt.assertToken(); err != nil {
		return nil, err
	}
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

	r := fvlt.realClient.NewRequest("PUT", "/v1/"+path)
	if err := r.SetJSONBody(data); err != nil {
		return nil, fmt.Errorf("failed to set request body to %+v: %v", data, err)
	}

	ctx := t.Local(addon.GoCtxKey).(context.Context)
	resp, err := fvlt.realClient.RawRequestWithContext(ctx, r)
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

// assertToken ensures that vault is only accessed if a token is set
func (fvlt *fakeVault) assertToken() (err error) {
	if fvlt.realClient.Token() == "" {
		return ErrNoToken
	}
	return
}

func (fvlt *fakeVault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		v, ok := fvlt.m[r.URL.Path]
		if !ok {
			// Fall back to real Vault client if fake key does not exist.
			ctx := context.Background()
			r := fvlt.realClient.NewRequest("GET", r.URL.Path)
			resp, err := fvlt.realClient.RawRequestWithContext(ctx, r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := resp.Error(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			bodyBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if _, err := w.Write(bodyBytes); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		m := json.RawMessage(fmt.Sprintf(`{"data": %s}`, v))
		b, err := m.MarshalJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if _, err := w.Write(b); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	case http.MethodPut:
		bs, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// If it's a PKI issue request, return a private key + cert.
		if strings.Contains(r.URL.Path, "/issue/") {
			m := json.RawMessage(`{"data":{"ca_chain":["ca0","ca1"],"certificate":"cert","issuing_ca":"ca","private_key":"privatekey"}}`)
			b, err := m.MarshalJSON()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if _, err := w.Write(b); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		fvlt.m[r.URL.Path] = string(bs)
	default:
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
	}
}

func NewFakeModule(fakeVault *fakeVault) (m starlark.HasAttrs, err error) {
	fakeVault.Module = &isopod.Module{
		Name: "vault",
		Attrs: starlark.StringDict{
			"read":     starlark.NewBuiltin("vault.read", fakeVault.vaultFakeReadFn),
			"read_raw": starlark.NewBuiltin("vault.read_raw", fakeVault.vaultFakeReadRawFn),
			"write":    starlark.NewBuiltin("vault.write", fakeVault.vaultFakeWriteFn),
			"exist":    starlark.NewBuiltin("vault.exist", fakeVault.vaultFakeExistFn),
		},
	}
	return fakeVault.Module, nil
}

// NewFake returns a new fake vault module for testing.
func NewFake() (m starlark.HasAttrs, closeFn func(), err error) {
	// Create a real Vault client for read fall back if key does not exist.
	vaultC, err := vaultapi.NewClient(&vaultapi.Config{
		Address: os.Getenv("VAULT_ADDR"),
	})
	vaultC.SetToken(os.Getenv("VAULT_TOKEN"))

	fakeVaultObj := &fakeVault{m: make(map[string]string), realClient: vaultC}
	s := httptest.NewTLSServer(fakeVaultObj)

	if err != nil {
		return nil, s.Close, fmt.Errorf("failed to initialize Vault client: %v", err)
	}

	module, _ := NewFakeModule(fakeVaultObj)
	return module, s.Close, nil
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
