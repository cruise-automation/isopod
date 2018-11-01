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

package kube

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stripe/skycfg"
	"go.starlark.net/starlark"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	util "github.com/cruise-automation/isopod/pkg/testing"
)

func TestQuantity(t *testing.T) {
	for _, tc := range []struct {
		desc         string
		qStr         string
		wantQuantity *resource.Quantity
		wantErr      string
	}{
		{
			desc:         "1GB of memory",
			qStr:         "kube.resource_quantity('5Gi')",
			wantQuantity: resource.NewQuantity(5*1024*1024*1024, resource.BinarySI),
		},
		{
			desc:         "1G of disk",
			qStr:         "kube.resource_quantity('5G')",
			wantQuantity: resource.NewQuantity(5*1000*1000*1000, resource.DecimalSI),
		},
		{
			desc:         "5.3 CPU cores",
			qStr:         "kube.resource_quantity('5300m')",
			wantQuantity: resource.NewMilliQuantity(5300, resource.DecimalSI),
		},
		{
			desc:    "Empty quantity",
			qStr:    "kube.resource_quantity('')",
			wantErr: "kube.resource_quantity: failed to parse quantity string: quantities must match the regular expression",
		},
		{
			desc:    "Invalid quantity",
			qStr:    "kube.resource_quantity('1GM')",
			wantErr: "kube.resource_quantity: failed to parse quantity string: unable to parse quantity's suffix",
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			v, _, err := util.Eval("kube", tc.qStr, nil, starlark.StringDict{"kube": &kubePackage{}})

			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if tc.wantErr == "" && gotErr != "" {
				t.Errorf("Unexpected error.\nWant: <nil>\nGot: %q", gotErr)
			}
			if !strings.Contains(gotErr, tc.wantErr) {
				t.Errorf("Unexpected error.\nWant fragment: %q\nGot: %q", tc.wantErr, gotErr)
			}

			if tc.wantQuantity == nil && v != nil {
				t.Fatalf("Expected nil return value. Got: %v", v)
			}

			if tc.wantQuantity != nil {
				m, ok := skycfg.AsProtoMessage(v)
				if !ok {
					t.Fatal("Return value is not a valid Protobuf message")
				}

				gotQuantity := m.(*resource.Quantity)
				if d := tc.wantQuantity.Cmp(*gotQuantity); d != 0 {
					t.Errorf("Unexpected parsed quantity: \nWant: %v\nGot: %v", tc.wantQuantity, gotQuantity)
				}
			}
		})
	}
}

func TestIntOrString(t *testing.T) {
	for _, tc := range []struct {
		desc      string
		tStr      string
		wantProto *intstr.IntOrString
		wantErr   string
	}{
		{
			desc:      "from_str: 10 percent",
			tStr:      "kube.from_str('10%')",
			wantProto: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
		},
		{
			desc:    "from_str: wrong type",
			tStr:    "kube.from_str(1)",
			wantErr: "kube.from_str: for parameter 1: got int, want string",
		},
		{
			desc:      "from_int: 42 int",
			tStr:      "kube.from_int(42)",
			wantProto: &intstr.IntOrString{IntVal: 42},
		},
		{
			desc:    "from_int: wrong type",
			tStr:    "kube.from_int('42')",
			wantErr: "kube.from_int: for parameter 1: got string, want int",
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			v, _, err := util.Eval("kube", tc.tStr, nil, starlark.StringDict{"kube": &kubePackage{}})

			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if tc.wantErr == "" && gotErr != "" {
				t.Errorf("Unexpected error.\nWant: <nil>\nGot: %q", gotErr)
			}
			if !strings.Contains(gotErr, tc.wantErr) {
				t.Errorf("Unexpected error.\nWant fragment: %q\nGot: %q", tc.wantErr, gotErr)
			}

			if tc.wantProto == nil && v != nil {
				t.Fatalf("Expected nil return value. Got: %v", v)
			}

			if tc.wantProto != nil {
				m, ok := skycfg.AsProtoMessage(v)
				if !ok {
					t.Fatal("Return value is not a valid Protobuf message")
				}

				gotProto := m.(*intstr.IntOrString)
				if diff := cmp.Diff(tc.wantProto, gotProto); diff != "" {
					t.Errorf("Unexpected *IntOrString (-want +got):\n%s", diff)
				}
			}
		})
	}
}
