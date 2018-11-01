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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	vault "github.com/hashicorp/vault/api"
	"go.starlark.net/starlark"

	isopod "github.com/cruise-automation/isopod/pkg"
)

type fakeVault struct {
	m map[string]string
}

func (h *fakeVault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		v, ok := h.m[r.URL.Path]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
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

		h.m[r.URL.Path] = string(bs)
	default:
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
	}
}

// NewFake returns a new fake vault module for testing.
func NewFake() (m starlark.HasAttrs, closeFn func(), err error) {
	s := httptest.NewTLSServer(&fakeVault{m: make(map[string]string)})

	c, err := vault.NewClient(&vault.Config{
		Address:    s.URL,
		HttpClient: s.Client(),
	})
	if err != nil {
		return nil, s.Close, err
	}
	return New(c, false /* dryRun */), s.Close, nil
}

// NewFakeWithServer returns a new fake vault module that uses s as its HTTP
// server.
func NewFakeWithServer(s *httptest.Server, dryRun bool) (*isopod.Module, error) {
	c, err := vault.NewClient(&vault.Config{
		Address:    s.URL,
		HttpClient: s.Client(),
	})
	if err != nil {
		return nil, err
	}
	return New(c, dryRun), nil
}
