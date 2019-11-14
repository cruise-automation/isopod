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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
	"go.starlark.net/starlark"
)

type fakeVault struct {
	realClient *vaultapi.Client
	m          map[string]string
}

func (h *fakeVault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		v, ok := h.m[r.URL.Path]
		if !ok {
			// Fall back to real Vault client if fake key does not exist.
			ctx := context.Background()
			r := h.realClient.NewRequest("GET", r.URL.Path)
			resp, err := h.realClient.RawRequestWithContext(ctx, r)
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

		h.m[r.URL.Path] = string(bs)
	default:
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
	}
}

// NewFake returns a new fake vault module for testing.
func NewFake() (m starlark.HasAttrs, closeFn func(), err error) {
	// Create a real Vault client for read fall back if key does not exist.
	vaultC, err := vaultapi.NewClient(&vaultapi.Config{
		Address: os.Getenv("VAULT_ADDR"),
	})
	vaultC.SetToken(os.Getenv("VAULT_TOKEN"))

	s := httptest.NewTLSServer(&fakeVault{m: make(map[string]string), realClient: vaultC})

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
	return New(c), s.Close, nil
}
