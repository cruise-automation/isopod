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

package addon

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"
)

// SkyCtx implements starlark.HasSetField.
type SkyCtx struct {
	Attrs starlark.StringDict
}

// Make sure SkyCtx implements starlark.HasSetField.
var _ starlark.HasSetField = (*SkyCtx)(nil)

// NewCtx returns new *SkyCtx.
func NewCtx() *SkyCtx {
	return &SkyCtx{
		Attrs: starlark.StringDict{},
	}
}

// String implements starlark.Value.String.
func (c *SkyCtx) String() string { return fmt.Sprintf("<ctx: %v>", c.Attrs) }

// Type implements starlark.Value.Type.
func (c *SkyCtx) Type() string { return "ctx" }

// Freeze implements starlark.Value.Freeze.
func (c *SkyCtx) Freeze() { c.Attrs.Freeze() }

// Truth implements starlark.Value.Truth.
// Returns true if object is non-empty.
func (c *SkyCtx) Truth() starlark.Bool { return len(c.Attrs) != 0 }

// Hash implements starlark.Value.Hash.
func (c *SkyCtx) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", c.Type()) }

// Attr implements starlark.HasAttrs.Attr.
func (c *SkyCtx) Attr(name string) (starlark.Value, error) {
	if val, ok := c.Attrs[name]; ok {
		return val, nil
	}
	return starlark.None, nil
}

// AttrNames implements starlark.HasAttrs.AttrNames.
func (c *SkyCtx) AttrNames() []string {
	var names []string
	for name := range c.Attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SetField implements starlark.HasSetField.SetField.
func (c *SkyCtx) SetField(name string, v starlark.Value) error {
	c.Attrs[name] = v
	return nil
}
