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

package isopod

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"
)

type Module struct {
	Name  string
	Attrs starlark.StringDict
}

// String implements starlark.Value.String.
func (m *Module) String() string { return fmt.Sprintf("<module: %s>", m.Name) }

// Type implements starlark.Value.Type.
func (m *Module) Type() string { return "<module>" }

// Freeze implements starlark.Value.Freeze.
func (m *Module) Freeze() {}

// Truth implements starlark.Value.Truth.
// Returns true if object is non-empty.
func (m *Module) Truth() starlark.Bool { return starlark.True }

// Hash implements starlark.Value.Hash.
func (m *Module) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", m.Type()) }

// Attr implements starlark.HasAttrs.Attr.
func (m *Module) Attr(name string) (starlark.Value, error) {
	v, ok := m.Attrs[name]
	if !ok {
		return nil, fmt.Errorf("<module: %s>: method name `%s' not found", m.Name, name)
	}
	return v, nil

}

// AttrNames implements starlark.HasAttrs.AttrNames.
func (m *Module) AttrNames() []string {
	var names []string
	for n := range m.Attrs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
