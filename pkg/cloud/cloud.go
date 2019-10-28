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

package cloud

import (
	"context"
	"fmt"

	log "github.com/golang/glog"
	"go.starlark.net/starlark"
	"k8s.io/client-go/rest"

	"github.com/cruise-automation/isopod/pkg/addon"
)

var (
	// asserts *AbstractKubeVendor implements starlark.HasAttrs interface.
	_ starlark.HasAttrs = (*AbstractKubeVendor)(nil)
)

// KubernetesVendor is the interface implemented by all Isopod-supported
// Kubernetes vendors.
type KubernetesVendor interface {
	// KubeConfig creates a rest config that will be used to connect to the cluster.
	KubeConfig(ctx context.Context) (*rest.Config, error)

	// AddonSkyCtx constructs a Starlark ctx object passed to each addon.
	// If additional context values could be passed to addon using the more input.
	AddonSkyCtx(more map[string]string) *addon.SkyCtx
}

// AbstractKubeVendor contains the common impl of all KubernetesVendor.
type AbstractKubeVendor struct {
	*addon.SkyCtx
	typeStr string
}

// NewAbstractKubeVendor creates a new AbstractKubeVendor.
func NewAbstractKubeVendor(typeStr string, requiredFields []string, kwargs []starlark.Tuple) (*AbstractKubeVendor, error) {
	required := map[string]struct{}{}
	for _, field := range requiredFields {
		required[field] = struct{}{}
	}
	kubeVendor := &AbstractKubeVendor{
		SkyCtx:  addon.NewCtx(),
		typeStr: typeStr,
	}
	for _, kwarg := range kwargs {
		k := string(kwarg[0].(starlark.String))
		v := kwarg[1]
		if _, ok := required[k]; ok {
			delete(required, k)
		}
		if err := kubeVendor.SetField(k, v); err != nil {
			return nil, fmt.Errorf("<%s> cannot process field `%v=%v`", typeStr, k, v)
		}
	}
	for unsetKey := range required {
		return nil, fmt.Errorf("<%s> requires field `%s'", typeStr, unsetKey)
	}
	return kubeVendor, nil
}

// String implements starlark.Value.String.
func (a *AbstractKubeVendor) String() string {
	return fmt.Sprintf("<%s: %v>", a.Type(), a.SkyCtx.Attrs)
}

// Type implements starlark.Value.Type.
func (a *AbstractKubeVendor) Type() string { return a.typeStr }

// AddonSkyCtx is part of the cloud.KubernetesVendor interface.
func (a *AbstractKubeVendor) AddonSkyCtx(more map[string]string) *addon.SkyCtx {
	for k, v := range more {
		if err := a.SkyCtx.SetField(k, starlark.String(v)); err != nil {
			log.Errorf("failed to set addon ctx `%s=%s': %v", k, v, err)
		}
	}
	return a.SkyCtx
}
