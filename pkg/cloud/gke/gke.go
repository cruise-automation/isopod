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

package gke

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"k8s.io/client-go/rest"

	"github.com/cruise-automation/isopod/pkg/addon"
	"github.com/cruise-automation/isopod/pkg/cloud"
)

const (
	// ClusterKey is the name of the cluster field.
	ClusterKey = "cluster"
	// ProjectKey is the name of the project field.
	ProjectKey = "project"
	// LocationKey is the name of the location field.
	LocationKey = "location"
	// UseInternalIPKey indicates if connecting API server via private endpoint
	UseInternalIPKey = "use_internal_ip"
)

var (
	// asserts *GKE implements starlark.HasAttrs interface.
	_ starlark.HasAttrs = (*GKE)(nil)
	// asserts *GKE implements cloud.KubernetesVendor interface.
	_ cloud.KubernetesVendor = (*GKE)(nil)

	// RequiredFields is the list of required fields to initialize a GKE target.
	RequiredFields = []string{ClusterKey, ProjectKey, LocationKey}
)

// GKE represents a GKE cluster. It includes critical information such as
// the cluster name, location, and project id, as well as other optional info.
type GKE struct {
	*cloud.AbstractKubeVendor
	svcAcctKeyFile, userAgent string
}

// NewGKEBuiltin creates a new GKE built-in.
func NewGKEBuiltin(svcAcctKeyFile, userAgent string) *starlark.Builtin {
	return starlark.NewBuiltin(
		"gke",
		func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			absKubeVendor, err := cloud.NewAbstractKubeVendor("gke", RequiredFields, kwargs)
			if err != nil {
				return nil, err
			}
			return &GKE{
				AbstractKubeVendor: absKubeVendor,
				svcAcctKeyFile:     svcAcctKeyFile,
				userAgent:          userAgent,
			}, nil
		},
	)
}

// KubeConfig is part of the cloud.KubernetesVendor interface.
func (g *GKE) KubeConfig(ctx context.Context) (*rest.Config, error) {
	cluster, location, project, useInternalIP, err := clpFromClusterCtx(g.SkyCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to extract cluster info from %v: %v", g, err)
	}
	return BuildKubeRestConfSACred(ctx, cluster, location, project, useInternalIP, g.svcAcctKeyFile, g.userAgent)
}

func stringFromValue(v starlark.Value) (string, error) {
	if v == nil {
		return "", errors.New("nil value")
	}
	s, ok := v.(starlark.String)
	if !ok {
		return "", fmt.Errorf("%v is not a starlark string (got a `%s')", v, v.Type())
	}
	return string(s), nil
}

func clpFromClusterCtx(c *addon.SkyCtx) (cluster, location, project, useInternalIP string, err error) {
	if cluster, err = stringFromValue(c.Attrs[ClusterKey]); err != nil {
		return
	}
	if location, err = stringFromValue(c.Attrs[LocationKey]); err != nil {
		return
	}
	if project, err = stringFromValue(c.Attrs[ProjectKey]); err != nil {
		return
	}
	if val := c.Attrs[UseInternalIPKey]; val != nil {
		if v, ok := val.(starlark.String); ok {
			useInternalIP = string(v)
		}
	}
	return
}
