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
	"strings"
)

// ParseCommaSeparatedParams slipts params in the form of
// "foo=bar,baz=qux" in to a map {"foo": "bar", "baz": "qux"}
func ParseCommaSeparatedParams(params string) (map[string]string, error) {
	parsed := map[string]string{}
	if params == "" {
		return parsed, nil
	}
	for _, p := range strings.Split(params, ",") {
		kv := strings.Split(p, "=")
		if len(kv) != 2 {
			return parsed, fmt.Errorf("invalid comma separated parameter (`%s'): %v", params, p)
		}
		parsed[kv[0]] = kv[1]
	}
	return parsed, nil
}
