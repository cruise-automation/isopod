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

package onprem

import (
	"testing"

	"go.starlark.net/starlark"

	util "github.com/cruise-automation/isopod/pkg/testing"
)

func TestOnPremBuiltin(t *testing.T) {
	for _, tc := range []struct {
		name    string
		expr    string
		wantVal starlark.Value
		wantErr error
	}{
		{
			name:    "reference first field",
			expr:    `onprem(cluster="minikube", env="dev").cluster`,
			wantVal: starlark.String("minikube"),
		},
		{
			name:    "reference second field",
			expr:    `onprem(cluster="minikube", env="dev").env`,
			wantVal: starlark.String("dev"),
		},
		{
			name:    "reference vaultkubeconfig field",
			expr:    `onprem(cluster="test", env="dev", vaultkubeconfig="secret/test").vaultkubeconfig`,
			wantVal: starlark.String("secret/test"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkgs := starlark.StringDict{"onprem": NewOnPremBuiltin("some-kubeconfig-file")}
			sval, _, err := util.Eval(t.Name(), tc.expr, nil, pkgs)
			if !util.ErrsEqual(err, tc.wantErr) {
				t.Fatalf("want error %v got %v", tc.wantErr, err)
			}
			if sval != tc.wantVal {
				t.Fatalf("want %v got %v", tc.wantVal, sval)
			}
		})
	}
}
