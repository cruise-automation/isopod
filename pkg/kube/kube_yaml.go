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
	"context"
	"fmt"
	"os"
	"strings"

	log "github.com/golang/glog"
	"go.starlark.net/starlark"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/cruise-automation/isopod/pkg/addon"
)

// DynamicClient used for applying dynamic resource manifests with no
// predefined protobufs such as CRDs.
type DynamicClient interface {
	Apply(t *starlark.Thread, name string, namespace string, data *starlark.List) (starlark.Value, error)
}

// kubePutYamlFn is entry point for `kube.put_yaml' callable.
func (m *kubePackage) kubePutYamlFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, namespace string
	data := &starlark.List{}
	unpacked := []interface{}{
		"name", &name,
		"data", &data,
		"namespace?", &namespace,
	}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, unpacked...); err != nil {
		return nil, fmt.Errorf("<%v>: %v", b.Name(), err)
	}

	val, err := m.Apply(t, name, namespace, data)
	if err != nil {
		return nil, fmt.Errorf("<%v>: %v", b.Name(), err)
	}

	return val, nil
}

func nameAndNamespace(name, namespace string, obj runtime.Object) (string, string, error) {
	a := meta.NewAccessor()

	objName, err := a.Name(obj)
	if err != nil {
		return "", "", err
	}
	if objName != "" {
		name = objName
	}

	objNs, err := a.Namespace(obj)
	if err != nil {
		return "", "", nil
	}

	if objNs != "" {
		namespace = objNs
	}

	return name, namespace, nil
}

func (m *kubePackage) Apply(t *starlark.Thread, name, namespace string, data *starlark.List) (starlark.Value, error) {
	for i := 0; i < data.Len(); i++ {
		maybeObj := data.Index(i)

		obj, gvk, err := decode([]byte(maybeObj.(starlark.String)))
		if err != nil {
			return nil, fmt.Errorf("item %d is not a YAML string (got: %s): %v", i, maybeObj.Type(), err)
		}

		sCtx := t.Local(addon.SkyCtxKey).(*addon.SkyCtx)
		// Override name and namespace if runtime.Object already set these.
		name, namespace, err = nameAndNamespace(name, namespace, obj)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve name and namespace for object %v/%s => %v", gvk.Kind, name, err)
		}

		r, err := newResourceForKind(m.dClient, name, namespace, "", *gvk)
		if err != nil {
			if _, ok := err.(*meta.NoKindMatchError); ok && m.dryRun {
				if err := printUnifiedDiff(os.Stdout, nil, obj, *gvk, maybeNamespaced(name, namespace), m.diffFilters); err != nil {
					return nil, err
				}
				return starlark.None, nil
			}
			return nil, fmt.Errorf("failed to map resource: %v", err)
		}
		if r.ClusterScoped {
			namespace = ""
		}

		if err := m.setMetadata(sCtx, name, namespace, obj); err != nil {
			return nil, fmt.Errorf("failed to validate/apply metadata for object %v/%s => %v", gvk.Kind, name, err)
		}

		ctx := t.Local(addon.GoCtxKey).(context.Context)
		if err := m.kubeUpdateYaml(ctx, r, obj); err != nil {
			return nil, err
		}
	}

	return starlark.None, nil
}

func parseUnstructuredStatus(un *unstructured.Unstructured) (details string, err error) {
	gvk := un.GroupVersionKind()
	if gvk.Kind == "Status" {
		ds, found, err := unstructured.NestedStringMap(un.Object, "details")
		if err != nil {
			return "", err
		}
		if !found {
			return "", nil
		}

		return fmt.Sprintf("%s%s `%s'", ds["kind"], maybeCore(ds["group"]), ds["name"]), nil
	}

	return fmt.Sprintf("%s%s `%s'", strings.ToLower(gvk.Kind), maybeCore(gvk.Group), maybeNamespaced(un.GetName(), un.GetNamespace())), nil
}

func (m *kubePackage) kubeUpdateYaml(ctx context.Context, r *apiResource, obj runtime.Object) error {
	live, found, err := m.kubePeek(ctx, m.Master+r.PathWithName())
	if err != nil {
		return err
	}
	if found {
		if err := mergeObjects(live, obj); err != nil {
			return err
		}
	}

	if m.dryRun {
		return printUnifiedDiff(os.Stdout, live, obj, r.GVK, maybeNamespaced(r.Name, r.Namespace), m.diffFilters)
	}

	var c dynamic.ResourceInterface = m.dynClient.Resource(r.GroupVersionResource())
	if r.Namespace != "" {
		c = c.(dynamic.NamespaceableResourceInterface).Namespace(r.Namespace)
	}

	if log.V(2) {
		s, err := renderObj(obj, &r.GVK, bool(log.V(3)) /* If --v=3, only return JSON. */, m.diffFilters)
		if err != nil {
			return fmt.Errorf("failed to render :live object for %v: %v", r, err)
		}

		log.Infof("%v:\n%s", r, s)
	}

	un, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}

	var resp *unstructured.Unstructured
	if found {
		resp, err = c.Update(&unstructured.Unstructured{Object: un}, metav1.UpdateOptions{})
	} else {
		resp, err = c.Create(&unstructured.Unstructured{Object: un}, metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}

	rMsg, err := parseUnstructuredStatus(resp)
	if err != nil {
		return err
	}

	log.Infof("%s updated", rMsg)

	return err
}
