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
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

// HumanReadableEvalError takes an error object returned by `starlark.Call` function,
// convert the error message to include stacktrace.
// If an error of any other type is passed in, it ignores and return the error object unmodified.
func HumanReadableEvalError(err error) error {
	if evalErr, ok := err.(*starlark.EvalError); ok {
		return errors.New(evalErr.Backtrace())
	}
	return err
}
