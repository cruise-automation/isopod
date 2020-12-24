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
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNewResource(t *testing.T) {
	for _, tc := range []struct {
		testName string

		name        string
		namespace   string
		apiGroup    string
		resource    string
		subresource string

		wantResource *apiResource
		wantErr      string
	}{
		{
			testName:    "apiGroup omitted",
			name:        "test-pod",
			namespace:   "ns",
			apiGroup:    "",
			resource:    "pod",
			subresource: "",

			wantResource: &apiResource{
				GVK: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				},
				Name:        "test-pod",
				Namespace:   "ns",
				Resource:    "pods",
				Subresource: "",
			},
		},
		{
			testName:    "apiGroup included",
			name:        "test-rs",
			namespace:   "ns",
			apiGroup:    "apps",
			resource:    "replicaset",
			subresource: "",

			wantResource: &apiResource{
				GVK: schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "ReplicaSet",
				},
				Name:        "test-rs",
				Namespace:   "ns",
				Resource:    "replicasets",
				Subresource: "",
			},
		},
		{
			testName:    "version included 1",
			name:        "test-crd",
			namespace:   "",
			apiGroup:    "apiextensions.k8s.io/v1",
			resource:    "customresourcedefinition",
			subresource: "",

			wantResource: &apiResource{
				GVK: schema.GroupVersionKind{
					Group:   "apiextensions.k8s.io",
					Version: "v1",
					Kind:    "CustomResourceDefinition",
				},
				Name:        "test-crd",
				Namespace:   "",
				Resource:    "customresourcedefinitions",
				Subresource: "",
			},
		},
		{
			testName:    "version included 2",
			name:        "test-crd",
			namespace:   "",
			apiGroup:    "apiextensions.k8s.io/v1beta1",
			resource:    "customresourcedefinition",
			subresource: "",

			wantResource: &apiResource{
				GVK: schema.GroupVersionKind{
					Group:   "apiextensions.k8s.io",
					Version: "v1beta1",
					Kind:    "CustomResourceDefinition",
				},
				Name:        "test-crd",
				Namespace:   "",
				Resource:    "customresourcedefinitions",
				Subresource: "",
			},
		},
	} {
		tc := tc
		t.Run(tc.testName, func(t *testing.T) {
			resource, err := newResource(
				fakeDiscovery(),
				tc.name,
				tc.namespace,
				tc.apiGroup,
				tc.resource,
				tc.subresource,
			)

			if tc.wantErr != "" {
				if err == nil {
					t.Errorf("Expect err `%s', got resource `%+v'", tc.wantErr, resource)
					return
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Expect err `%s', got err `%s'", tc.wantErr, err)
					return
				}
			}
			if err != nil {
				t.Errorf("Expect resource `%+v', got err `%s'", resource, err)
				return
			}

			if !reflect.DeepEqual(resource, tc.wantResource) {
				r1, _ := json.MarshalIndent(tc.wantResource, "", "\t")
				r2, _ := json.MarshalIndent(resource, "", "\t")
				t.Errorf("Expect resource `%s', got resource `%s'", string(r1), string(r2))
				return
			}
		})
	}
}
