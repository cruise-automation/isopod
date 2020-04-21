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

// Package kube implements "kube" starlark built-in which renders and applies
// Kubernetes objects.
package kube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/stripe/skycfg"
	"go.starlark.net/starlark"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/cruise-automation/isopod/pkg/addon"
	"github.com/cruise-automation/isopod/pkg/util"
)

const (
	namespaceResrc = "namespace"
	apiGroupKW     = "api_group"
)

// kubePackage implements Kubernetes package that can be imported by plugin
// code.
type kubePackage struct {
	dClient     discovery.DiscoveryInterface
	dynClient   dynamic.Interface
	httpClient  *http.Client
	dryRun      bool
	force       bool
	diff        bool
	diffFilters []string
	// host:port of the master endpoint.
	Master string
}

// New returns a new skaylark.HasAttrs object for kube package.
func New(
	addr string,
	d discovery.DiscoveryInterface,
	dynC dynamic.Interface,
	c *http.Client,
	dryRun, force, diff bool,
	diffFilters []string,
) starlark.HasAttrs {

	return &kubePackage{
		dClient:     d,
		dynClient:   dynC,
		httpClient:  c,
		Master:      addr,
		dryRun:      dryRun,
		force:       force,
		diff:        diff,
		diffFilters: diffFilters,
	}
}

// String implements starlark.Value.String.
func (m *kubePackage) String() string { return "<pkg: kube>" }

// Type implements starlark.Value.Type.
func (m *kubePackage) Type() string { return "kube" }

// Freeze implements starlark.Value.Freeze.
func (m *kubePackage) Freeze() {}

// Truth implements starlark.Value.Truth.
// Returns true if object is non-empty.
func (m *kubePackage) Truth() starlark.Bool { return starlark.True }

// Hash implements starlark.Value.Hash.
func (m *kubePackage) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", m.Type()) }

const (
	kubeDeleteMethod           = "delete"
	kubeFromIntMethod          = "from_int"
	kubeFromStrMethod          = "from_str"
	kubeGetMethod              = "get"
	kubeExistsMethod           = "exists"
	kubePutMethod              = "put"
	kubePutYamlMethod          = "put_yaml"
	kubeResourceQuantityMethod = "resource_quantity"
)

// Attr implement starlark.HasAttrs.Attr.
func (m *kubePackage) Attr(name string) (starlark.Value, error) {
	switch name {
	case kubeDeleteMethod:
		return starlark.NewBuiltin("kube."+kubeDeleteMethod, m.kubeDeleteFn), nil
	case kubeFromIntMethod:
		return starlark.NewBuiltin("kube."+kubeFromIntMethod, fromIntFn), nil
	case kubeFromStrMethod:
		return starlark.NewBuiltin("kube."+kubeFromStrMethod, fromStringFn), nil
	case kubeGetMethod:
		return starlark.NewBuiltin("kube."+kubeGetMethod, m.kubeGetFn), nil
	case kubeExistsMethod:
		return starlark.NewBuiltin("kube."+kubeExistsMethod, m.kubeExistsFn), nil
	case kubePutMethod:
		return starlark.NewBuiltin("kube."+kubePutMethod, m.kubePutFn), nil
	case kubePutYamlMethod:
		return starlark.NewBuiltin("kube."+kubePutYamlMethod, m.kubePutYamlFn), nil
	case kubeResourceQuantityMethod:
		return starlark.NewBuiltin("kube."+kubeResourceQuantityMethod, resourceQuantityFn), nil
	}
	return nil, fmt.Errorf("unexpected attr: %s", name)
}

// AttrNames implement starlark.HasAttrs.AttrNames.
func (m *kubePackage) AttrNames() []string {
	return []string{
		kubeGetMethod,
		kubeExistsMethod,
		kubePutMethod,
		kubeDeleteMethod,
		kubeResourceQuantityMethod,
		kubePutYamlMethod,
	}
}

// ctxAnnotationName is the key of a context annotation added to all
// Isopod-provisioned objects.
const ctxAnnotationKey = "isopod.getcruise.com/context"

// setMetadata sets metadata fields on the obj.
func (m *kubePackage) setMetadata(tCtx *addon.SkyCtx, name, namespace string, obj runtime.Object) error {
	a := meta.NewAccessor()

	objName, err := a.Name(obj)
	if err != nil {
		return err
	}
	if objName != "" && objName != name {
		return fmt.Errorf("name=`%s' argument does not match object's .metadata.name=`%s'", name, objName)
	}
	if err := a.SetName(obj, name); err != nil {
		return err
	}

	if namespace != "" { // namespace is optional argument.
		objNs, err := a.Namespace(obj)
		if err != nil {
			return err
		}
		if objNs != "" && objNs != namespace {
			return fmt.Errorf("namespace=`%s' argument does not match object's .metadata.namespace=`%s'", namespace, objNs)
		}

		if err := a.SetNamespace(obj, namespace); err != nil {
			return err
		}
	}

	ls, err := a.Labels(obj)
	if err != nil {
		return err
	}
	if ls == nil {
		ls = map[string]string{}
	}

	ls["heritage"] = "isopod"
	if err := a.SetLabels(obj, ls); err != nil {
		return err
	}

	as, err := a.Annotations(obj)
	if err != nil {
		return err
	}
	if as == nil {
		as = map[string]string{}
	}

	bs, err := json.Marshal(tCtx.Attrs)
	if err != nil {
		return err
	}
	as[ctxAnnotationKey] = string(bs)
	return a.SetAnnotations(obj, as)
}

func getResourceAndName(resArg starlark.Tuple) (resource, name string, err error) {
	resourceArg, ok := resArg[0].(starlark.String)
	if !ok {
		err = errors.New("expected string for resource")
		return
	}
	resource = string(resourceArg)
	nameArg, ok := resArg[1].(starlark.String)
	if !ok {
		err = errors.New("expected string for resource name")
		return
	}
	name = string(nameArg)
	return
}

// kubePutFn is entry point for `kube.put' callable.
// TODO(dmitry-ilyevskiy): Return Status object from the response as Starlark dict.
func (m *kubePackage) kubePutFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, namespace, apiGroup, subresource string
	data := &starlark.List{}
	unpacked := []interface{}{
		"name", &name,
		"data", &data,
		"namespace?", &namespace,
		// TODO(dmitry-ilyevskiy): Remove this when https://github.com/stripe/skycfg/issues/14
		// is resolved upstream.
		"api_group?", &apiGroup,
		"subresource?", &subresource,
	}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, unpacked...); err != nil {
		return nil, fmt.Errorf("<%v>: %v", b.Name(), err)
	}

	for i := 0; i < data.Len(); i++ {
		maybeMsg := data.Index(i)
		msg, ok := skycfg.AsProtoMessage(maybeMsg)
		if !ok {
			return nil, fmt.Errorf("<%v>: item %d is not a protobuf type. got: %s", b.Name(), i, maybeMsg.Type())
		}

		sCtx := t.Local(addon.SkyCtxKey).(*addon.SkyCtx)
		if err := m.setMetadata(sCtx, name, namespace, msg.(runtime.Object)); err != nil {
			return nil, fmt.Errorf("<%v>: failed to validate/apply metadata for object %d => %v: %v", b.Name(), i, maybeMsg.Type(), err)
		}

		r, err := newResourceForMsg(m.dClient, name, namespace, apiGroup, subresource, msg)
		if err != nil {
			return nil, fmt.Errorf("<%v>: failed to map resource: %v", b.Name(), err)
		}

		ctx := t.Local(addon.GoCtxKey).(context.Context)
		if err := m.kubeUpdate(ctx, r, msg); err != nil {
			return nil, fmt.Errorf("<%v>: %v", b.Name(), err)
		}
	}

	return starlark.None, nil
}

// kubeDeleteFn is entry point for `kube.delete' callable.
// TODO(dmitry-ilyevskiy): Return Status object from the response as Starlark dict.
func (m *kubePackage) kubeDeleteFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("<%v>: positional args not supported by `kube.delete': %v", b.Name(), args)
	}

	if len(kwargs) < 1 {
		return nil, fmt.Errorf("<%v>: expected at least <resource>=<name>", b.Name())
	}

	resource, name, err := getResourceAndName(kwargs[0])
	if err != nil {
		return nil, fmt.Errorf("<%v>: %s", b.Name(), err.Error())
	}

	// If resource is not namespace itself (special case) attempt to parse
	// namespace out of the arg value.
	var namespace string
	if resource != namespaceResrc {
		ss := strings.Split(name, "/")
		if len(ss) > 1 {
			namespace = ss[0]
			name = ss[1]
		}
	}

	// Optional api_group argument.
	var apiGroup starlark.String
	var foreground starlark.Bool
	for _, kv := range kwargs[1:] {
		switch string(kv[0].(starlark.String)) {
		case apiGroupKW:
			var ok bool
			if apiGroup, ok = kv[1].(starlark.String); !ok {
				return nil, fmt.Errorf("<%v>: expected string value for `%s' arg, got: %s", b.Name(), apiGroupKW, kv[1].Type())
			}
		case "foreground":
			var ok bool
			if foreground, ok = kv[1].(starlark.Bool); !ok {
				return nil, fmt.Errorf("<%v>: expected string value for `foreground' arg, got: %s", b.Name(), kv[1].Type())
			}
		default:
			return nil, fmt.Errorf("<%v>: expected `api_group' or `foreground', got: %v=%v", b.Name(), kv[0], kv[1])
		}
	}

	r, err := newResource(m.dClient, name, namespace, string(apiGroup), resource, "")
	if err != nil {
		return nil, fmt.Errorf("<%v>: failed to map resource: %v", b.Name(), err)
	}

	ctx := t.Local(addon.GoCtxKey).(context.Context)
	if err := m.kubeDelete(ctx, r, bool(foreground)); err != nil {
		return nil, fmt.Errorf("<%v>: %v", b.Name(), err)
	}

	return starlark.None, nil
}

// kubeGetFn is an entry point for `kube.get` built-in.
func (m *kubePackage) kubeGetFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("<%v>: positional args not supported: %v", b.Name(), args)
	}

	if len(kwargs) < 1 {
		return nil, fmt.Errorf("<%v>: expected <resource>=<name>", b.Name())
	}

	resource, name, err := getResourceAndName(kwargs[0])
	if err != nil {
		return nil, fmt.Errorf("<%v>: %s", b.Name(), err.Error())
	}

	// If resource is not namespace itself (special case), attempt to parse
	// namespace out of the arg value.
	var namespace string
	if resource != namespaceResrc {
		ss := strings.Split(name, "/")
		if len(ss) > 1 {
			namespace = ss[0]
			name = ss[1]
		}
	}

	// Optional api_group argument.
	var apiGroup starlark.String
	var wait time.Duration
	var wantJSON bool
	for _, kv := range kwargs[1:] {
		switch string(kv[0].(starlark.String)) {
		case apiGroupKW:
			var ok bool
			if apiGroup, ok = kv[1].(starlark.String); !ok {
				return nil, fmt.Errorf("<%v>: expected string value for `%s' arg, got: %s", b.Name(), apiGroupKW, kv[1].Type())
			}
		case "wait":
			durStr, ok := kv[1].(starlark.String)
			if !ok {
				return nil, fmt.Errorf("<%v>: expected string value for `wait' arg, got: %s", b.Name(), kv[1].Type())
			}

			var err error
			if wait, err = time.ParseDuration(string(durStr)); err != nil {
				return nil, fmt.Errorf("<%v>: failed to parse duration value: %v", b.Name(), err)
			}
		case "json":
			bv, ok := kv[1].(starlark.Bool)
			if !ok {
				return nil, fmt.Errorf("<%v>: expected boolean value for `json' arg, got: %s", b.Name(), kv[1].Type())
			}
			wantJSON = bool(bv)
		default:
			return nil, fmt.Errorf("<%v>: expected one of [ api_group | wait | json ] args, got: %v=%v", b.Name(), kv[0], kv[1])
		}
	}

	r, err := newResource(m.dClient, name, namespace, string(apiGroup), resource, "")
	if err != nil {
		return nil, fmt.Errorf("<%v>: failed to map resource: %v", b.Name(), err)
	}

	ctx := t.Local(addon.GoCtxKey).(context.Context)
	obj, err := m.kubeGet(ctx, r, wait)
	if err != nil {
		return nil, fmt.Errorf("<%v>: failed to get %s%s `%s': %v", b.Name(), resource, maybeCore(string(apiGroup)), name, err)
	}

	if wantJSON {
		un, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, fmt.Errorf("<%v>: failed to convert %s%s `%s' to unstructured JSON: %v", b.Name(), resource, maybeCore(string(apiGroup)), name, err)
		}
		return util.ValueFromNestedMap(un)
	}

	p, ok := obj.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("<%v>: could not convert object to proto: %v", b.Name(), obj)
	}

	return skycfg.NewProtoMessage(p), nil
}

// kubeExistsFn is an entry point for `kube.exists` built-in.
func (m *kubePackage) kubeExistsFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return starlark.False, fmt.Errorf("<%v>: positional args not supported: %v", b.Name(), args)
	}

	if len(kwargs) < 1 {
		return starlark.False, fmt.Errorf("<%v>: expected <resource>=<name>", b.Name())
	}

	resource, name, err := getResourceAndName(kwargs[0])
	if err != nil {
		return nil, fmt.Errorf("<%v>: %s", b.Name(), err.Error())
	}

	// If resource is not namespace itself (special case), attempt to parse
	// namespace out of the arg value.
	var namespace string
	if resource != namespaceResrc {
		ss := strings.Split(name, "/")
		if len(ss) > 1 {
			namespace = ss[0]
			name = ss[1]
		}
	}

	// Optional api_group argument.
	var apiGroup starlark.String
	var wait time.Duration
	for _, kv := range kwargs[1:] {
		switch string(kv[0].(starlark.String)) {
		case apiGroupKW:
			var ok bool
			if apiGroup, ok = kv[1].(starlark.String); !ok {
				return starlark.False, fmt.Errorf("<%v>: expected string value for `%s' arg, got: %s", b.Name(), apiGroupKW, kv[1].Type())
			}
		case "wait":
			durStr, ok := kv[1].(starlark.String)
			if !ok {
				return starlark.False, fmt.Errorf("<%v>: expected string value for `wait' arg, got: %s", b.Name(), kv[1].Type())
			}

			var err error
			if wait, err = time.ParseDuration(string(durStr)); err != nil {
				return starlark.False, fmt.Errorf("<%v>: failed to parse duration value: %v", b.Name(), err)
			}
		default:
			return starlark.False, fmt.Errorf("<%v>: expected one of [ api_group | wait ] args, got: %v=%v", b.Name(), kv[0], kv[1])
		}
	}

	r, err := newResource(m.dClient, name, namespace, string(apiGroup), resource, "")
	if err != nil {
		return starlark.False, fmt.Errorf("<%v>: failed to map resource: %v", b.Name(), err)
	}

	ctx := t.Local(addon.GoCtxKey).(context.Context)
	_, err = m.kubeGet(ctx, r, wait)
	if err == ErrNotFound {
		return starlark.False, nil
	} else if err != nil {
		return starlark.False, err
	}

	return starlark.True, nil
}

var k8sProtoMagic = []byte("k8s\x00")

// marshal wraps msg into runtime.Unknown object and prepends magic sequence
// to conform with Kubernetes protobuf content type.
func marshal(msg proto.Message, gvk schema.GroupVersionKind) ([]byte, error) {
	msgBytes, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	v, k := gvk.ToAPIVersionAndKind()
	unknownBytes, err := proto.Marshal(&runtime.Unknown{
		TypeMeta: runtime.TypeMeta{
			APIVersion: v,
			Kind:       k,
		},
		Raw: msgBytes,
	})
	if err != nil {
		return nil, err
	}
	return append(k8sProtoMagic, unknownBytes...), nil
}

var decodeFn = Codecs.UniversalDeserializer().Decode

func decode(raw []byte) (runtime.Object, *schema.GroupVersionKind, error) {
	obj, gvk, err := decodeFn(raw, nil, nil)
	if err == nil {
		return obj, gvk, nil
	}
	if !runtime.IsNotRegisteredError(err) {
		return nil, nil, err
	}

	// When the input is already a json, this just returns it as-is.
	j, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, nil, err
	}

	return unstructured.UnstructuredJSONScheme.Decode(j, nil, nil)
}

// parseHTTPResponse parses response body to extract runtime.Object
// and HTTP return code.
// Returns details message on success and error on failure (includes HTTP
// response codes not in 2XX).
func parseHTTPResponse(r *http.Response) (obj runtime.Object, details string, err error) {
	raw, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read body (response code: %d): %v", r.StatusCode, err)
	}

	log.V(2).Infof("Response raw data: %s", raw)
	obj, gvk, err := decode(raw)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse json object (response code: %d): %v", r.StatusCode, err)
	}

	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return nil, "", fmt.Errorf("%s (response code: %d)", apierrors.FromObject(obj).Error(), r.StatusCode)
	}

	if s, ok := obj.(*metav1.Status); ok {
		d := s.Details
		if d == nil {
			return obj, s.Message, nil
		}
		return obj, fmt.Sprintf("%s%s `%s", d.Kind, d.Group, d.Name), nil
	}

	in, ok := obj.(metav1.Object)
	if ok {
		return obj, fmt.Sprintf("%s%s `%s'", strings.ToLower(gvk.Kind), maybeCore(gvk.Group), maybeNamespaced(in.GetName(), in.GetNamespace())), nil
	} else if _, ok := obj.(metav1.ListInterface); ok {
		return obj, fmt.Sprintf("%s%s'", strings.ToLower(gvk.Kind), maybeCore(gvk.Group)), nil
	}
	return nil, "", fmt.Errorf("returned object does not implement `metav1.Object` or `metav1.ListInterface`: %v", obj)
}

// kubePeek checks if object by url exists in Kubernetes.
func (m *kubePackage) kubePeek(ctx context.Context, url string) (obj runtime.Object, found bool, err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}

	log.V(1).Infof("GET to %s", url)

	resp, err := m.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, false, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}

	obj, _, err = parseHTTPResponse(resp)
	if err != nil {
		return nil, false, err
	}
	return obj, true, nil
}

var ErrUpdateImmutable = errors.New("cannot update immutable. Use -force to delete and recreate")

func ErrImmutableRessource(attribute string, obj runtime.Object) error {
	return fmt.Errorf("failed to update %s of resource %s: %w", attribute, obj.GetObjectKind().GroupVersionKind().String(), ErrUpdateImmutable)
}

// mergeObjects merges the fields from the live object to the new
// object such as resource version and clusterIP.
// TODO(jon.yucel): Instead of selectively picking fields, holisticly
// solving this problem requires three-way merge implementation.
func mergeObjects(live, obj runtime.Object) error {
	// Service's clusterIP needs to be re-set to the value provided
	// by controller or mutation will be denied.
	if liveSvc, ok := live.(*corev1.Service); ok {
		svc := obj.(*corev1.Service)
		svc.Spec.ClusterIP = liveSvc.Spec.ClusterIP

		gotPort := liveSvc.Spec.HealthCheckNodePort
		wantPort := svc.Spec.HealthCheckNodePort
		// If port is set (non-zero) and doesn't match the existing port (also non-zero), error out.
		if wantPort != 0 && gotPort != 0 && wantPort != gotPort {
			return ErrImmutableRessource(".spec.healthCheckNodePort", obj)
		}
		svc.Spec.HealthCheckNodePort = gotPort
	}

	if liveClusterRoleBinding, ok := live.(*rbacv1.ClusterRoleBinding); ok {
		clusterRoleBinding := obj.(*rbacv1.ClusterRoleBinding)
		if liveClusterRoleBinding.RoleRef.APIGroup != clusterRoleBinding.RoleRef.APIGroup ||
			liveClusterRoleBinding.RoleRef.Kind != clusterRoleBinding.RoleRef.Kind ||
			liveClusterRoleBinding.RoleRef.Name != clusterRoleBinding.RoleRef.Name {
			return ErrImmutableRessource("roleRef", obj)
		}
	}

	// Set metadata.resourceVersion for updates as required by
	// Kubernetes API (http://go/k8s-concurrency).
	if gotRV := live.(metav1.Object).GetResourceVersion(); gotRV != "" {
		obj.(metav1.Object).SetResourceVersion(gotRV)
	}

	return nil
}

// maybeRecreate can be called to check if a resource can be updated or
// is immutable and needs recreation.
// It evaluates if resource should be forcefully recreated. In that case
// the resource will be deleted and recreated. If the -force flag is not
// enabled and an immutable resource should be updated, an error is thrown
// and no resources will get deleted.
func maybeRecreate(ctx context.Context, live, obj runtime.Object, m *kubePackage, r *apiResource) error {
	err := mergeObjects(live, obj)
	if errors.Is(errors.Unwrap(err), ErrUpdateImmutable) && m.force {
		if m.dryRun {
			fmt.Fprintf(os.Stdout, "\n\n**WARNING** %s %s is immutable and will be deleted and recreated.\n", strings.ToLower(r.GVK.Kind), maybeNamespaced(r.Name, r.Namespace))
		}
		// kubeDelete() already properly handles a dry run, so the resource won't be deleted if -force is set, but in dry run mode
		if err := m.kubeDelete(ctx, r, true); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

// kubeUpdate creates or overwrites object in Kubernetes.
// Path is computed based on msg type, name and (optional) namespace (these must
// not conflict with name and namespace set in object metadata).
func (m *kubePackage) kubeUpdate(ctx context.Context, r *apiResource, msg proto.Message) error {
	uri := r.PathWithName()
	live, found, err := m.kubePeek(ctx, m.Master+uri)
	if err != nil {
		return err
	}

	method := http.MethodPut
	if found {
		// Reset uri in case subresource update is requested.
		uri = r.PathWithSubresource()
		if err := maybeRecreate(ctx, live, msg.(runtime.Object), m, r); err != nil {
			return err
		}
	} else { // Object doesn't exist so create it.
		if r.Subresource != "" {
			return errors.New("parent resource does not exist")
		}

		method = http.MethodPost
		uri = r.Path()
	}

	bs, err := marshal(msg, r.GVK)
	if err != nil {
		return err
	}

	url := m.Master + uri
	// Set body type as marshaled Protobuf.
	// TODO(dmitry-ilyevskiy): Will not work for CRDs (only json encoding
	// is supported) so the user will have to indicate this is a
	// non-standard type (or we could deduce that ourselves).
	contentType := "application/vnd.kubernetes.protobuf"
	req, err := http.NewRequest(method, url, bytes.NewReader(bs))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)

	log.V(1).Infof("%s to %s", method, url)

	if log.V(2) {
		s, err := renderObj(msg.(runtime.Object), &r.GVK, bool(log.V(3)) /* If --v=3, only return JSON. */, m.diffFilters)
		if err != nil {
			return fmt.Errorf("failed to render :live object for %s: %v", r.String(), err)
		}

		log.Infof("%s:\n%s", r.String(), s)
	}

	if m.diff {
		if err := printUnifiedDiff(os.Stdout, live, msg.(runtime.Object), r.GVK, maybeNamespaced(r.Name, r.Namespace), m.diffFilters); err != nil {
			return err
		}
	}

	if m.dryRun {
		return printUnifiedDiff(os.Stdout, live, msg.(runtime.Object), r.GVK, maybeNamespaced(r.Name, r.Namespace), m.diffFilters)
	}

	resp, err := m.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}

	_, rMsg, err := parseHTTPResponse(resp)
	if err != nil {
		return err
	}

	actionMsg := "created"
	if method == http.MethodPut {
		actionMsg = "updated"
	}
	log.Infof("%s %s", rMsg, actionMsg)

	return nil
}

// kubeDelete deletes namespace/name resource in Kubernetes.
// Attempts to deduce GroupVersionResource from apiGroup (optional) and resource
// strings. Fails if multiple matches found.
func (m *kubePackage) kubeDelete(_ context.Context, r *apiResource, foreground bool) error {
	var c dynamic.ResourceInterface = m.dynClient.Resource(r.GroupVersionResource())
	if r.Namespace != "" {
		c = c.(dynamic.NamespaceableResourceInterface).Namespace(r.Namespace)
	}

	delPolicy := metav1.DeletePropagationBackground
	if foreground {
		delPolicy = metav1.DeletePropagationForeground
	}

	log.V(1).Infof("DELETE to %s", m.Master+r.PathWithName())

	if m.dryRun {
		return nil
	}

	if err := c.Delete(r.Name, &metav1.DeleteOptions{
		PropagationPolicy: &delPolicy,
	}); err != nil {
		return err
	}

	log.Infof("%v deleted", r)

	return nil
}

// waitRetryInterval is a duration between consecutive get retries.
const waitRetryInterval = 1 * time.Second

var ErrNotFound = errors.New("not found")

// kubeGet attempts to read namespace/name resource from an apiGroup from API
// Server.
// If object is not present will retry every waitRetryInterval up to wait (only
// tries once if wait is zero).
func (m *kubePackage) kubeGet(ctx context.Context, r *apiResource, wait time.Duration) (runtime.Object, error) {
	url := m.Master + r.PathWithName()
	retryCh := make(chan interface{}, 1)
	retryCh <- struct{}{} // Seed the channel so that we don't wait initially.
	var waitDone <-chan time.Time
	if wait != 0 {
		waitDone = time.After(wait)
	}

	for {
		select {
		case retryCh <- time.After(waitRetryInterval):
		case <-retryCh:
			obj, ok, err := m.kubePeek(ctx, url)
			if err != nil {
				return nil, err
			}

			if ok {
				return obj, nil
			}

			if waitDone == nil {
				return nil, ErrNotFound
			}

		case <-waitDone:
			return nil, ErrNotFound
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// not reachable
}
