// Copyright 2020 Cruise LLC
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
	"io/ioutil"
	"os"
	"strings"
	"testing"

	_ "github.com/golang/glog"
	"github.com/google/go-cmp/cmp"
)

func TestLoadFilterFile(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write []string
		read  []string
	}{
		{
			name: "full set",
			write: []string{
				"metadata.annotations[\"isopod.getcruise.com/context\"]",
				"metadata.annotations[\"deployment.kubernetes.io/revision\"]",
				"metadata.annotations[\"autoscaling.alpha.kubernetes.io/conditions\"]",
				"metadata.annotations[\"cloud.google.com/neg-status\"]",
				"spec.template.spec.serviceAccount",
			},
			read: []string{
				"metadata.annotations[\"isopod.getcruise.com/context\"]",
				"metadata.annotations[\"deployment.kubernetes.io/revision\"]",
				"metadata.annotations[\"autoscaling.alpha.kubernetes.io/conditions\"]",
				"metadata.annotations[\"cloud.google.com/neg-status\"]",
				"spec.template.spec.serviceAccount",
			},
		},
		{
			name: "full set",
			write: []string{
				"metadata.annotations[\"isopod.getcruise.com/context\"]",
				"metadata.annotations[\"deployment.kubernetes.io/revision\"]",
				"metadata.annotations[\"autoscaling.alpha.kubernetes.io/conditions\"]",
				"", // blank lines removed
				"metadata.annotations[\"cloud.google.com/neg-status\"]",
				"# this line is a comment", // comment lines removed
				"spec.template.spec.serviceAccount",
				"", // trailing new lines removed
			},
			read: []string{
				"metadata.annotations[\"isopod.getcruise.com/context\"]",
				"metadata.annotations[\"deployment.kubernetes.io/revision\"]",
				"metadata.annotations[\"autoscaling.alpha.kubernetes.io/conditions\"]",
				"metadata.annotations[\"cloud.google.com/neg-status\"]",
				"spec.template.spec.serviceAccount",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {

			content := []byte(strings.Join(tc.write, "\n"))

			tmpfile, err := ioutil.TempFile("", "isopod-kube-diff-filters-test")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpfile.Name()) // clean up

			if _, err := tmpfile.Write(content); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpfile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			got, err := LoadFilterFile(tmpfile.Name())
			if err != nil {
				t.Fatalf("Failed to load diff filters from file: %v", err)
			}

			if !cmp.Equal(tc.read, got) {
				t.Errorf("Unexpected filters loaded: \nWant: %v\nGot: %v", tc.read, got)
			}
		})
	}
}
