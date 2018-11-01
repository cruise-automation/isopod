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
	"errors"

	"github.com/cruise-automation/isopod/pkg/store"
)

// Config is used to create a new Runtime.
type Config struct {
	// EntryFile is the path to the main Starlark file that the
	// Isopod runtime initializes to. It must contains ClustersStarFunc
	// and AddonsStarFunc.
	EntryFile string

	// GCPSvcAcctKeyFile is the path to the Google Service Account
	// Credential file. It is used to authenticate with GKE clusters.
	GCPSvcAcctKeyFile string

	// UserAgent is the UserAgent name used by Isopod to communicate with all
	// Kubernetes masters.
	UserAgent string

	// KubeConfigPath is the path to the kubeconfig file on the local machine.
	// It is used to authenticate with self-managed or on-premise Kubernetes.
	KubeConfigPath string

	// DryRun is true if commands run in dry-run mode, which does not alter
	// the live configuration in the cluster. It will print YAML diff against
	// live objects in cluster.
	DryRun bool

	// Store is the storage to keep all rollout status.
	Store store.Store
}

// Validate checks if all required fields are set.
func Validate(c *Config) error {
	if c.EntryFile == "" {
		return errors.New("runtime.Config.EntryFile cannot be empty")
	}
	if c.UserAgent == "" {
		return errors.New("runtime.Config.UserAgent cannot be empty")
	}
	return nil
}
