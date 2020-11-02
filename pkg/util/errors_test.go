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
	"fmt"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func makeEvalErr() *starlark.EvalError {
	file := "/file.ipd"
	return &starlark.EvalError{
		Msg: "invalid call of non-function (string)",
		CallStack: starlark.CallStack{
			{
				Name: "foo",
				Pos:  syntax.MakePosition(&file, 1, 1),
			},
			{
				Name: "bar",
				Pos:  syntax.MakePosition(&file, 1, 1),
			},
		},
	}
}

func TestHumanReadableEvalError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		wantErr error
	}{
		{
			name: "normal",
			err:  makeEvalErr(),
			wantErr: fmt.Errorf(`Traceback (most recent call last):
  /file.ipd:1:1: in foo
  /file.ipd:1:1: in bar
Error: invalid call of non-function (string)`),
		},
		{
			name:    "nil error",
			err:     nil,
			wantErr: nil,
		},
		{
			name:    "unrelated error",
			err:     fmt.Errorf("foo"),
			wantErr: fmt.Errorf("foo"),
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotErr := HumanReadableEvalError(tc.err)
			if gotErr == nil || tc.wantErr == nil {
				if gotErr != tc.wantErr {
					t.Errorf("Expect %s, got %s", tc.wantErr, gotErr)
					return
				}
				return
			}
			if gotErr.Error() != tc.wantErr.Error() {
				t.Errorf("Expect error message `%s'\ngot `%s'", tc.wantErr.Error(), gotErr.Error())
				return
			}
		})
	}
}
