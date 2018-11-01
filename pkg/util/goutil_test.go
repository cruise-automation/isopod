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
	"errors"
	"reflect"
	"testing"
)

func TestParseCommaSeparatedParams(t *testing.T) {
	for _, tc := range []struct {
		name, params   string
		expectedOutput map[string]string
		expectedErr    error
	}{
		{
			name:   "success",
			params: "foo=bar,baz=qux",
			expectedOutput: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name:           "empty",
			params:         "",
			expectedOutput: map[string]string{},
		},
		{
			name:        "failure",
			params:      "foo=bar,baz",
			expectedErr: errors.New("invalid comma separated parameter (`foo=bar,baz'): baz"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCommaSeparatedParams(tc.params)
			if tc.expectedErr != nil {
				if err.Error() != tc.expectedErr.Error() {
					t.Errorf("Expect error\n%v\nGot error\n%v", tc.expectedErr, err)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.expectedOutput) {
				t.Errorf("Expect\n%v\nGot\n%v", tc.expectedOutput, got)
			}
		})
	}
}
