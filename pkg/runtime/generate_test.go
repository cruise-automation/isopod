// Copyright 2020 GM Cruise LLC
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

package runtime

import (
	"fmt"
	"io/ioutil"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGenerate(t *testing.T) {
	testdataPath := "testdata"
	testcases := map[string]struct {
		inputPath string
		wantPath  string
	}{
		"Generate yaml": {
			inputPath: path.Join(testdataPath, "clusterrolebinding.yaml"),
			wantPath:  path.Join(testdataPath, "clusterrolebinding.ipd"),
		},
		"Generate json": {
			inputPath: path.Join(testdataPath, "deployment.json"),
			wantPath:  path.Join(testdataPath, "deployment.ipd"),
		},
		"Generate CRD": {
			inputPath: path.Join(testdataPath, "crd.yaml"),
			wantPath:  path.Join(testdataPath, "crd.ipd"),
		},
	}

	for name, test := range testcases {
		t.Run(name, func(t *testing.T) {
			got := ""
			out = func(format string, a ...interface{}) { got = fmt.Sprintf(format, a...) }
			err := Generate(test.inputPath)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := ioutil.ReadFile(test.wantPath)
			if d := cmp.Diff(string(want), got); d != "" {
				t.Errorf("Unexpected output (-want, +got):\n%s", d)
			}
		})
	}
}
