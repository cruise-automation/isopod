package vault

import (
	"go.starlark.net/starlark"

	"fmt"
)

// fakeValues implements starlark.Mapping and starlark which provides dict-like fake interface.
// Is meant to be used to simulate a fakeVault read which returns a fake value for any key
type fakeValues struct{}

// String implements starlark.Value.String.
// Produces stable output.
func (fv *fakeValues) String() string {
	out := `map["value":"fake"]`
	return out
}

// Type implements starlark.Value.Type.
func (fv *fakeValues) Type() string { return "vault: secret" }

// Freeze implements starlark.Value.Freeze.
func (fv *fakeValues) Freeze() {}

// Truth implements starlark.Value.Truth.
// Always returns true because the dict is always non-empty
func (fv *fakeValues) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.Hash.
// Returns error since dicts are unhashable in Python.
func (fv *fakeValues) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", fv.Type()) }

// Get implements starlark.Mapping.Get.
// Assumes k is a starlark.String. Always returns a fake string value (a starlark.String).
func (fv *fakeValues) Get(k starlark.Value) (v starlark.Value, found bool, err error) {
	_, ok := k.(starlark.String)
	if !ok {
		return nil, false, fmt.Errorf("want string key, got: %v", k.Type())
	}
	val := starlark.String("fake")
	return val, true, nil
}

// Len implements starlark.Sequence.Len.
func (fv *fakeValues) Len() int { return 1 }
