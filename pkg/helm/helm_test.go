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

package helm

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	util "github.com/cruise-automation/isopod/pkg/testing"
)

type FakeDynamicClient struct {
	err  error
	data *starlark.List
}

func (f *FakeDynamicClient) Apply(t *starlark.Thread, name, namespace string, data *starlark.List) (starlark.Value, error) {
	f.data = data
	if f.err != nil {
		return nil, f.err
	}

	return starlark.None, nil
}

func TestHelmPackage(t *testing.T) {

	globalValues := `{
        "global": {
            "priorityClassName": "cluster-critical",
		},
	}`

	values := `{
		"pilot": {
			"replicaCount": 3,
			"traceSampling": 50,
			"image": "docker.io/istio/pilot:v1.2.3",
		},
    }`
	overlayValues := `{
		"pilot": {
			"traceSampling": 75,
		},
    }`
	for _, tc := range []struct {
		name         string
		expr         string
		wantRendered *starlark.List
		wantErr      error
		skip         bool
	}{
		{
			name:    "Missing required arg",
			expr:    `helm.apply(release_name="helm-test")`,
			wantErr: errors.New("helm.apply: missing argument for chart"),
		},
		{
			name:    "Unsupported remote chart",
			expr:    `helm.apply(release_name="helm-test", chart="istio")`,
			wantErr: errors.New("helm.apply: remote repositories are not supported yet"),
		},
		{
			name:    "Invalid chart source",
			expr:    `helm.apply(release_name="helm-test", chart="//testdata/istio/helm-test")`,
			wantErr: errors.New("helm.apply: stat testdata/istio/helm-test: no such file or directory"),
		},
		{
			name:    "Missing required value",
			expr:    `helm.apply(release_name="helm-test", chart="//../../testdata/istio/helm-test")`,
			wantErr: errors.New("helm.apply: render error in"),
		},
		{
			name:    "Success",
			expr:    `helm.apply(release_name="helm-test", chart="//../../testdata/istio/helm-test", namespace="istio-system", values=[` + globalValues + `, ` + values + `, ` + overlayValues + `])`,
			wantErr: nil,
			wantRendered: starlark.NewList(
				[]starlark.Value{
					starlark.String(expectedDeployment),
					starlark.String(expectedMesh),
				},
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}

			fc := &FakeDynamicClient{}
			pkgs := starlark.StringDict{"helm": New(fc, "")}
			_, _, gotErr := util.Eval(t.Name(), tc.expr, nil, pkgs)
			if gotErr != nil {
				if tc.wantErr == nil {
					t.Fatalf("Unexpected error. Want: nil\nGot: %s", gotErr)
				}
				if !strings.HasPrefix(gotErr.Error(), tc.wantErr.Error()) {
					t.Fatalf("Unexpected error.\nWant: %s\nGot: %s", tc.wantErr, gotErr)
				}
			}
			if tc.wantRendered != nil {
				if fc.data != nil && fc.data.Len() != tc.wantRendered.Len() {
					t.Fatalf("Unexpected rendered list.\nWant: %d\nGot: %d", tc.wantRendered.Len(), fc.data.Len())
				}
				if !reflect.DeepEqual(fc.data, tc.wantRendered) {
					t.Fatalf("Unexpected rendered list.\nWant: %s\nGot: %s", tc.wantRendered, fc.data)
				}
			}
		})

	}
}

const (
	expectedDeployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: istio-pilot
  namespace: istio-system
  labels:
    app: pilot
    release: helm-test
    istio: pilot
spec:
  replicas: 3
  selector:
    matchLabels:
      istio: pilot
  template:
    metadata:
      labels:
        app: pilot
        istio: pilot
      annotations:
        sidecar.istio.io/inject: "false"
    spec:
      serviceAccountName: istio-pilot-service-account
      priorityClassName: "cluster-critical"
      containers:
        - name: discovery
          image: "docker.io/istio/pilot:v1.2.3"
          imagePullPolicy: Always
          args:
          - "discovery"
          - --monitoringAddr=:15014
          ports:
          - containerPort: 8080
          env:
          - name: PILOT_TRACE_SAMPLING
            value: "75"`

	expectedMesh = `apiVersion: "authentication.istio.io/v1alpha1"
kind: "MeshPolicy"
metadata:
  name: "default"
  namespace: istio-system
  labels:
    release: helm-test
spec:
  peers:
  - mtls:
      mode: PERMISSIVE`
)
