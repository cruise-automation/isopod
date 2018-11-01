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

package testing

import (
	"bufio"
	"bytes"
	"context"

	"go.starlark.net/starlark"

	"github.com/cruise-automation/isopod/pkg/addon"
)

// Eval executes src in its own Starlark thread and returns a resulting Starlark
// value and a scanner that can be used to read eval output produced with a
// "print" built-in.
func Eval(name string, src, runCtx interface{}, env starlark.StringDict) (starlark.Value, *bufio.Scanner, error) {
	b := new(bytes.Buffer)
	sc := bufio.NewScanner(b)
	t := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) {
			b.Write([]byte(msg))
		},
	}
	ctx := context.Background()
	t.SetLocal(addon.GoCtxKey, ctx)
	t.SetLocal(addon.SkyCtxKey, runCtx)
	v, err := starlark.Eval(t, name, src, env)
	return v, sc, err
}

// ErrsEqual checks if two errors are equal.
func ErrsEqual(errA, errB error) bool {
	if (errA == nil) != (errB == nil) { // xor
		return false
	}
	if errA != nil { // both errA and errB are not nil
		return errA.Error() == errB.Error()
	}
	return true
}
