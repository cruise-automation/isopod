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
	"context"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/cruise-automation/isopod/pkg/cloud"
	"github.robot.car/cruise/isopod/pkg/vault"
)

var (
	// asserts *GKE implements starlark.HasAttrs interface.
	_ starlark.HasAttrs = (*OnPrem)(nil)
	// asserts *GKE implements cloud.KubernetesVendor interface.
	_ cloud.KubernetesVendor = (*OnPrem)(nil)
)

// OnPrem represents a on-premise cluster.
type OnPrem struct {
	*cloud.AbstractKubeVendor
	kubeConfigFile string
}

// NewOnPremBuiltin creates a new OnPrem built-in.
func NewOnPremBuiltin(kubeConfigFile string) *starlark.Builtin {
	return starlark.NewBuiltin(
		"onprem",
		func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			absKubeVendor, err := cloud.NewAbstractKubeVendor("onprem", nil, kwargs)
			if err != nil {
				return nil, err
			}
			return &OnPrem{
				AbstractKubeVendor: absKubeVendor,
				kubeConfigFile:     kubeConfigFile,
			}, nil
		},
	)
}

// KubeConfig is part of the cloud.KubernetesVendor interface.
func (o *OnPrem) KubeConfig(ctx context.Context) (*rest.Config, error) {
	kubeConfigVaultPath := o.AbstractKubeVendor.AddonSkyCtx(
		map[string]string{}).Attrs["vaultkubeconfig"].(starlark.String).String()

	// Should only access vault kubeconfig if the kubeConfigFile flag was not set
	// and the vaultkubeconfig attribute is set in the star config.
	if len(kubeConfigVaultPath) > 0 && len(o.kubeConfigFile) < 1 {
		// Remove the surrounding quotes from the Starlark string
		kubeConfigVaultPath = strings.Trim(kubeConfigVaultPath, `"`)

		value, err := vault.ReadVaultPath(kubeConfigVaultPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read kubeconfig vault path: %v", err)
		}

		kubeconfig := []byte(value)

		return clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	}
	return clientcmd.BuildConfigFromFlags("", o.kubeConfigFile)
}
