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
	"fmt"
	"path/filepath"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"go.starlark.net/starlark"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/engine"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/timeconv"
	"sigs.k8s.io/yaml"

	isopod "github.com/cruise-automation/isopod/pkg"
	"github.com/cruise-automation/isopod/pkg/kube"
)

const yamlSeparator = "---"

type helmPackage struct {
	*isopod.Module
	client  kube.DynamicClient
	baseDir string
}

// New returns a new starlark.HasAttrs object for helm package.
func New(c kube.DynamicClient, baseDir string) starlark.HasAttrs {
	h := &helmPackage{
		client:  c,
		baseDir: baseDir,
	}

	h.Module = &isopod.Module{
		Name: "helm",
		Attrs: starlark.StringDict{
			"apply": starlark.NewBuiltin("helm.apply", h.helmApplyFn),
		},
	}

	return h
}

func (h *helmPackage) helmApplyFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, namespace, chartSource string
	values := &starlark.List{}
	unpacked := []interface{}{
		"release_name", &name,
		"chart", &chartSource,
		"namespace?", &namespace,
		"values?", &values,
	}

	if err := starlark.UnpackArgs(b.Name(), args, kwargs, unpacked...); err != nil {
		return nil, err
	}
	if strings.HasPrefix(chartSource, "//") {
		chartSource = strings.Replace(chartSource, "//", "", 1)
		chartSource = filepath.Join(h.baseDir, chartSource)
	} else if !filepath.IsAbs(chartSource) {
		// TODO(jon.yucel): add remote repository support
		return nil, fmt.Errorf("%s: remote repositories are not supported yet <%s>", b.Name(), chartSource)
	}

	resources, err := h.render(name, namespace, chartSource, values)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", b.Name(), err)
	}

	val, err := h.client.Apply(t, "", namespace, starlark.NewList(resources))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", b.Name(), err)
	}

	return val, nil
}

func (h *helmPackage) render(name, namespace, chartSource string, values *starlark.List) ([]starlark.Value, error) {
	chrt, err := chartutil.Load(chartSource)
	if err != nil {
		return nil, err
	}

	merged, err := mergeValues(values)
	if err != nil {
		return nil, err
	}

	config := &chart.Config{Raw: string(merged), Values: map[string]*chart.Value{}}

	options := chartutil.ReleaseOptions{
		Name:      name,
		Time:      timeconv.Now(),
		Namespace: namespace,
	}

	vals, err := chartutil.ToRenderValuesCaps(chrt, config, options, nil)
	if err != nil {
		return nil, err
	}

	files, err := engine.New().Render(chrt, vals)
	if err != nil {
		return nil, err
	}

	l := []starlark.Value{}
	for filename, f := range files {
		if strings.HasSuffix(filename, "NOTES.txt") {
			continue
		}
		resources := strings.Split(f, yamlSeparator)
		for _, r := range resources {
			r = strings.TrimSpace(r)
			// Helm might lead to some yaml sections with only comment lines.
			// This is a naive check to ignore those files.
			if r != "" && strings.Contains(r, "kind:") && strings.Contains(r, "apiVersion:") {
				l = append(l, starlark.String(r))
			}
		}
	}

	return l, nil
}

func mergeValues(values *starlark.List) ([]byte, error) {
	var merged []byte
	if values.Len() == 0 {
		return merged, nil
	}

	merged, err := yaml.YAMLToJSON([]byte(values.Index(0).String()))
	if err != nil {
		return nil, err
	}

	for i := 1; i < values.Len(); i++ {
		res, err := yaml.YAMLToJSON([]byte(values.Index(i).String()))
		if err != nil {
			return nil, err
		}

		merged, err = jsonpatch.MergePatch(merged, res)
		if err != nil {
			return nil, err
		}
	}
	return merged, nil
}
