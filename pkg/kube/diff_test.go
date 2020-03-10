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
	"bytes"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func multiline(s ...string) string {
	return strings.Join(s, "\n")
}

func TestDiff(t *testing.T) {
	now := metav1.Now()
	for _, tc := range []struct {
		name       string
		live, head runtime.Object
		wantDiff   string
		wantErr    error
	}{
		{
			name: "No diff",
			live: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: now,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 80,
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{
					StartTime: &now,
				},
			},
			head: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
			wantDiff: "\n*** pod.v1 `foobar' ***\n",
		},
		{
			name: "Pod diff",
			live: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: now,
					Annotations: map[string]string{
						"isopod.getcruise.com/context":      "any value",
						"deployment.kubernetes.io/revision": "3",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
			head: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
							Ports: []corev1.ContainerPort{
								{
									Name:          "https",
									ContainerPort: 443,
								},
							},
						},
					},
				},
			},
			wantDiff: multiline("",
				"*** pod.v1 `foobar' ***",
				"--- live",
				"+++ head",
				"@@ -3,12 +3,12 @@",
				" spec:",
				"   containers:",
				"   - name: nginx",
				"     image: nginx:latest",
				"     ports:",
				"-    - name: http",
				"-      containerPort: 80",
				"+    - name: https",
				"+      containerPort: 443",
				"       protocol: TCP",
				"     resources: {}",
				"     terminationMessagePath: /dev/termination-log",
				"     terminationMessagePolicy: File",
				"     imagePullPolicy: Always",
				""),
		},
	} {
		var rw bytes.Buffer

		t.Run(tc.name, func(t *testing.T) {
			diffFilters := []string{
				`metadata.annotations["isopod.getcruise.com/context"]`,
				`metadata.annotations["deployment.kubernetes.io/revision"]`,
				`metadata.annotations["autoscaling.alpha.kubernetes.io/conditions"]`,
				`metadata.annotations["cloud.google.com/neg-status"]`,
				`spec.template.spec.serviceAccount`,
			}
			err := printUnifiedDiff(&rw, tc.live, tc.head, tc.live.(runtime.Object).GetObjectKind().GroupVersionKind(), "foobar", diffFilters)
			if err != nil {
				t.Fatalf("Failed to write diff: %v", err)
			}

			bs, gotErr := ioutil.ReadAll(&rw)
			if err != nil {
				t.Fatalf("Failed to read from output buffer: %v", err)
			}
			gotDiff := string(bs)

			if !cmp.Equal(tc.wantErr, gotErr) {
				t.Fatalf("Unexpected error.\nWant: %v\ngot: %v", tc.wantErr, gotErr)
			}

			if tc.wantDiff != gotDiff {
				t.Errorf("Unexpected diff.\nWant:\n%s\nGot:\n%s", tc.wantDiff, gotDiff)
			}
		})
	}
}
