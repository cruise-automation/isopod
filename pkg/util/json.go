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

package util

import (
	"encoding/json"
	"fmt"
	"sort"

	"go.starlark.net/starlark"
)

// values implements starlark.Mapping and starlark which provides dict-like interface.
type values struct {
	v map[starlark.String]starlark.Value
}

// String implements starlark.Value.String.
// Produces stable output.
func (vs *values) String() string {
	var keys []string
	for k := range vs.v {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	out := "map["
	for i, k := range keys {
		out += fmt.Sprintf("%q:%v", k, vs.v[starlark.String(k)])
		if i < len(keys)-1 {
			out += " "
		}
	}
	out += "]"
	return out
}

// Type implements starlark.Value.Type.
func (vs *values) Type() string { return "vault: secret" }

// Freeze implements starlark.Value.Freeze.
func (vs *values) Freeze() {}

// Truth implements starlark.Value.Truth.
// Return true if map is non-empty.
func (vs *values) Truth() starlark.Bool { return len(vs.v) > 0 }

// Hash implements starlark.Value.Hash.
// Returns error since dicts are unhashable in Python.
func (vs *values) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", vs.Type()) }

// Get implements starlark.Mapping.Get.
// Assumes k is a starlark.String. Returns a corresponding value v (also a
// starlark.String), if found.
func (vs *values) Get(k starlark.Value) (v starlark.Value, found bool, err error) {
	s, ok := k.(starlark.String)
	if !ok {
		return nil, false, fmt.Errorf("want string key, got: %v", k.Type())
	}

	r, ok := vs.v[s]
	if !ok {
		return nil, false, nil
	}

	return r, true, nil
}

// Len implements starlark.Sequence.Len.
func (vs *values) Len() int { return len(vs.v) }

// keysIterator implemented starlark.Iterator.
type keysIterator struct {
	keys  []starlark.String
	index int
}

// Next implements starlark.Iterator.Next.
func (iter *keysIterator) Next(p *starlark.Value) bool {
	if iter.index >= len(iter.keys) {
		return false
	}
	*p = iter.keys[iter.index]
	iter.index++
	return true
}

// Next implements starlark.Iterator.Done.
func (iter *keysIterator) Done() { iter.index = 0 }

// Iterator implements starlark.Iterable.Iterator.
func (vs *values) Iterator() starlark.Iterator {
	keys := make([]starlark.String, len(vs.v))
	for k := range vs.v {
		keys = append(keys, k)
	}
	return &keysIterator{keys: keys}
}

// ValueFromJSON converts JSON value to starlark.Value.
func ValueFromJSON(v interface{}) (starlark.Value, error) {
	if v == nil {
		return starlark.None, nil
	}

	switch t := v.(type) {
	case map[string]interface{}:
		return ValueFromNestedMap(t)
	case []interface{}:
		vs := &starlark.List{}
		for i, item := range t {
			vv, err := ValueFromJSON(item)
			if err != nil {
				return nil, fmt.Errorf("failed to convert item to Starlark type [%d]=%v: %v", i, item, err)
			}

			if err = vs.Append(vv); err != nil {
				return nil, fmt.Errorf("failed to append item `%s' to list: %v", vv, err)
			}
		}
		return vs, nil
	case string:
		return starlark.String(t), nil
	case float64:
		return starlark.Float(t), nil
	case bool:
		return starlark.Bool(t), nil
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return nil, err
		}
		return starlark.Float(f), nil
	default:
		return nil, fmt.Errorf("unsupported JSON data type: %T", t)
	}
	// not reachable
}

// ValueFromNestedMap converts nested JSON map oject to starlark.Value.
func ValueFromNestedMap(m map[string]interface{}) (starlark.Value, error) {
	out := make(map[starlark.String]starlark.Value, len(m))
	for k, v := range m {
		sv, err := ValueFromJSON(v)
		if err != nil {
			return nil, err
		}
		out[starlark.String(k)] = sv
	}
	return &values{v: out}, nil
}
