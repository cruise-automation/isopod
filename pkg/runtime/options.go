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
	"fmt"
	"net/http"
	"reflect"
	"regexp"

	gogo_proto "github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/proto"
	vapi "github.com/hashicorp/vault/api"
	"github.com/stripe/skycfg"
	"go.starlark.net/starlark"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	// Proto imports for type registration.
	_ "k8s.io/api/batch/v1"
	_ "k8s.io/api/core/v1"
	_ "k8s.io/api/storage/v1"
	_ "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	// Plugin imports for auth.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/cruise-automation/isopod/pkg/helm"
	"github.com/cruise-automation/isopod/pkg/kube"
	"github.com/cruise-automation/isopod/pkg/vault"
)

// Option is an interface that applies (enables) a specific option to a set of
// options.
type Option interface {
	apply(*options) error
}

type options struct {
	dryRun  bool
	force   bool
	noSpin  bool
	pkgs    starlark.StringDict
	addonRe *regexp.Regexp
}

type fnOption func(*options) error

func (fn fnOption) apply(opts *options) error { return fn(opts) }

// WithNoSpin option disables terminal spinner (UI).
func WithNoSpin() Option {
	return fnOption(func(opts *options) error {
		opts.noSpin = true
		return nil
	})
}

// WithVault returns an Option that enables "vault" package.
func WithVault(c *vapi.Client) Option {
	return fnOption(func(opts *options) error {
		opts.pkgs["vault"] = vault.New(c)
		if opts.dryRun {
			// TODO use var from cmd entrypoint
			opts.pkgs["vault"], _, _ = vault.NewFake(false)
		}
		return nil
	})
}

// protoRegistry implements UNSTABLE proto registry API (subject to change:
// https://github.com/golang/protobuf/issues/364).
type protoRegistry struct{}

// UnstableProtoMessageType implements lookup from full protobuf message name
// to a Go type of the generated message struct.
func (*protoRegistry) UnstableProtoMessageType(name string) (reflect.Type, error) {
	if t := proto.MessageType(name); t != nil {
		return t, nil
	}
	if t := gogo_proto.MessageType(name); t != nil {
		return t, nil
	}
	return nil, nil
}

// UnstableEnumValueMap implements lookup from go-protobuf enum name to the
// name->value map.
func (*protoRegistry) UnstableEnumValueMap(name string) map[string]int32 {
	if ev := proto.EnumValueMap(name); ev != nil {
		return ev
	}
	if ev := gogo_proto.EnumValueMap(name); ev != nil {
		return ev
	}
	return nil
}

// WithKube returns an Option that enables "kube" package.
func WithKube(c *rest.Config, diff bool, diffFilters []string) Option {
	return fnOption(func(opts *options) error {
		dC := discovery.NewDiscoveryClientForConfigOrDie(c)

		t, err := rest.TransportFor(c)
		if err != nil {
			return err
		}

		dynC, err := dynamic.NewForConfig(c)
		if err != nil {
			return err
		}

		opts.pkgs["kube"] = kube.New(c.Host, dC, dynC, &http.Client{Transport: t}, opts.dryRun, opts.force, diff, diffFilters)
		pkgs := skycfg.UnstablePredeclaredModules(&protoRegistry{})
		for name, pkg := range pkgs {
			opts.pkgs[name] = pkg
		}

		return nil
	})
}

func WithHelm(baseDir string) Option {
	return fnOption(func(opts *options) error {
		v, ok := opts.pkgs["kube"]
		if !ok {
			return fmt.Errorf("kube package must be initialized first")
		}

		d, ok := v.(kube.DynamicClient)
		if !ok {
			return fmt.Errorf("package doesn't implement kube.DynamicClient")
		}

		opts.pkgs["helm"] = helm.New(d, baseDir)

		return nil
	})
}

// WithAddonRegex returns an Option that filters addons using supplied regex.
func WithAddonRegex(r *regexp.Regexp) Option {
	return fnOption(func(opts *options) error {
		opts.addonRe = r
		return nil
	})
}
