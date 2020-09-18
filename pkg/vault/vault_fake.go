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
	realClient         *vaultapi.Client
	realClientFallBack bool
	m                  map[string]string
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
		if !ok && fvlt.realClientFallBack {
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
			"read": starlark.NewBuiltin("vault.read", fakeVault.vaultFakeReadFn),
		},
	}
	return fakeVault.Module, nil
}

// NewFake returns a new fake vault module for testing.
func NewFake(realClientFallBack bool) (m starlark.HasAttrs, closeFn func(), err error) {
	// Create a real Vault client for read fall back if key does not exist.
	vaultC, err := vaultapi.NewClient(&vaultapi.Config{
		Address: os.Getenv("VAULT_ADDR"),
	})
	vaultC.SetToken(os.Getenv("VAULT_TOKEN"))

	fakeVaultObj := &fakeVault{m: make(map[string]string), realClient: vaultC, realClientFallBack: realClientFallBack}

	s := httptest.NewTLSServer(fakeVaultObj)

	if err != nil {
		return nil, s.Close, fmt.Errorf("failed to initialize Vault client: %v", err)
	}

	c, err := vaultapi.NewClient(&vaultapi.Config{
		Address:    s.URL,
		HttpClient: s.Client(),
	})
	if err != nil {
		return nil, s.Close, err
	}
	c.SetToken("fake_token")
	if !realClientFallBack {
		module, _ := NewFakeModule(fakeVaultObj)
		return module, s.Close, nil
	}
	return New(c), s.Close, nil
}
