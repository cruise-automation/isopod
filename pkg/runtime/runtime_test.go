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

package runtime

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.starlark.net/starlark"

	"github.com/cruise-automation/isopod/pkg/cloud"
	"github.com/cruise-automation/isopod/pkg/store"
)

// storeStub implements Store interface for no-op store.
type storeStub struct{}

func (storeStub) CreateRollout() (*store.Rollout, error) { return &store.Rollout{}, nil }

func (storeStub) PutAddonRun(id store.RolloutID, _ *store.AddonRun) (store.RunID, error) {
	return "", nil
}

func (storeStub) CompleteRollout(id store.RolloutID) error { return nil }

func (storeStub) GetLive() (*store.Rollout, bool, error) {
	return nil, false, nil
}

func (storeStub) GetRollout(id store.RolloutID) (r *store.Rollout, found bool, err error) {
	return nil, false, nil
}

func TestForEachCluster(t *testing.T) {
	ctx := context.Background()

	runtime, err := New(&Config{
		EntryFile:         "../../testdata/main.ipd",
		GCPSvcAcctKeyFile: "some-sa-key",
		UserAgent:         "Isopod",
		KubeConfigPath:    "kubeconfig",
		Store:             storeStub{},
		DryRun:            false,
		Force:             false,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := runtime.Load(ctx); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name           string
		selector       map[string]string
		expectClusters []string
	}{
		{
			name:           "env=dev",
			selector:       map[string]string{"env": "dev"},
			expectClusters: []string{"paas-dev", "minikube"},
		},
		{
			name:           "empty selector",
			selector:       map[string]string{},
			expectClusters: []string{"paas-dev", "paas-staging", "paas-prod", "minikube"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotClusters []string
			if err := runtime.ForEachCluster(ctx, tc.selector, func(k8sVendor cloud.KubernetesVendor) {
				c := k8sVendor.AddonSkyCtx(tc.selector)
				gotClusters = append(gotClusters, string(c.Attrs["cluster"].(starlark.String)))

				if err := runtime.Run(ctx, InstallCommand, c); err != nil {
					t.Errorf("Run failed: %v", err)
				}
			}); err != nil {
				t.Fatal(err)
			}

			if d := cmp.Diff(tc.expectClusters, gotClusters); d != "" {
				t.Errorf("Unexpected cluster (-want, +got):\n%s", d)
			}
		})
	}
}
