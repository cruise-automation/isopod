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
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	gogo_proto "github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/stripe/skycfg"
	"go.starlark.net/starlark"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	appsv1 "k8s.io/api/apps/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cruise-automation/isopod/pkg/addon"
	util "github.com/cruise-automation/isopod/pkg/testing"
)

const noneValue = "None"

func statusWithDetails(group, kind, name, msg string) *metav1.Status {
	return &metav1.Status{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Status",
			APIVersion: "v1",
		},
		Message: msg,
		Details: &metav1.StatusDetails{
			Kind:  strings.ToLower(kind),
			Group: group,
			Name:  name,
		},
	}
}

func cmpObjMeta(wantMeta *metav1.ObjectMeta, got []byte) (diff string, err error) {
	un := &apiruntime.Unknown{}
	if err := proto.Unmarshal(got, un); err != nil {
		return "", err
	}

	var gotMeta metav1.ObjectMeta
	switch un.TypeMeta.Kind {
	case "Pod":
		pod := &corev1.Pod{}
		if err := proto.Unmarshal(un.Raw, pod); err != nil {
			return "", err
		}
		gotMeta = pod.ObjectMeta
	case "Service":
		svc := &corev1.Service{}
		if err := proto.Unmarshal(un.Raw, svc); err != nil {
			return "", err
		}
		gotMeta = svc.ObjectMeta
	default:
		return "", fmt.Errorf("unexpected kind: %v", un.TypeMeta.Kind)

	}

	return cmp.Diff(*wantMeta, gotMeta), nil
}

// fakeKubernetes implements fake Kubernetes API endpoint (https://).
// Returns fake HTTP client, a fake server URL and a cleanup function.
func fakeKubernetes(
	t *testing.T,
	gotObj apiruntime.Object,
	wantURLs []string,
	wantJSON bool,
	wantObjMeta *metav1.ObjectMeta,
	wantDeletion metav1.DeletionPropagation) (*http.Client, string, func()) {

	urlIdx := 0
	ks := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotObj == nil && r.Method == http.MethodGet {
			http.Error(w, "", http.StatusNotFound)
			return
		}

		wantURL := ""
		if len(wantURLs) != 0 {
			wantURL, wantURLs = wantURLs[0], wantURLs[1:]
		}
		if wantURL != r.URL.Path {
			http.Error(w, "", http.StatusBadRequest)
			t.Errorf("Unexpected URL[%d]:\nWant: %v\nGot: %v", urlIdx, wantURL, r.URL.Path)
			s := statusWithDetails("", "", "", "bad url")
			bs, _ := apiruntime.Encode(unstructured.UnstructuredJSONScheme, s)
			write(w, bs)
			return
		}
		urlIdx++

		switch r.Method {
		case http.MethodGet:
			bs, err := apiruntime.Encode(unstructured.UnstructuredJSONScheme, gotObj)
			if err != nil {
				msg := fmt.Sprintf("failed to encode response object: %v", err)
				http.Error(w, msg, http.StatusBadRequest)
				t.Error(msg)
				return
			}
			write(w, bs)
		case http.MethodDelete:
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "no body", http.StatusBadRequest)
				t.Error("no body")
				return
			}

			decode := scheme.Codecs.UniversalDeserializer().Decode
			obj, _, err := decode(body, nil, nil)
			if err != nil {
				http.Error(w, "error decoding body", http.StatusBadRequest)
				t.Errorf("error decoding body: %v", err)
				return
			}

			delOpts, ok := obj.(*metav1.DeleteOptions)
			if !ok {
				http.Error(w, "object not a *metav1.DeleteOptions", http.StatusBadRequest)
				t.Errorf("object not a `*metav1.DeleteOptions': %v", delOpts)
				return
			}

			if pp := delOpts.PropagationPolicy; pp == nil || *pp != wantDeletion {
				http.Error(w, "unexpected propagation policy", http.StatusBadRequest)
				t.Errorf("unexpected propagation policy. want: %v, got: %v", wantDeletion, *pp)
				return
			}

			bs, err := apiruntime.Encode(unstructured.UnstructuredJSONScheme, gotObj)
			if err != nil {
				msg := fmt.Sprintf("failed to encode response object: %v", err)
				http.Error(w, msg, http.StatusBadRequest)
				t.Error(msg)
				return
			}
			write(w, bs)
		case http.MethodPut, http.MethodPost:
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "no body", http.StatusBadRequest)
				t.Error("no body")
				return
			}

			if wantJSON {
				obj, _, err := decodeFn(body, nil, nil)
				if err != nil {
					http.Error(w, "decode error", http.StatusBadRequest)
					t.Errorf("decode error: %v", err)
					return
				}

				gotMetaV := reflect.ValueOf(obj).Elem().FieldByName("ObjectMeta")
				if !gotMetaV.IsValid() {
					http.Error(w, "no .metadata", http.StatusBadRequest)
					t.Error("no .metadata")
					return
				}
				gotMeta := gotMetaV.Interface().(metav1.ObjectMeta)

				if d := cmp.Diff(*wantObjMeta, gotMeta); d != "" {
					msg := fmt.Sprintf("Mismatching `ObjectMeta': (-want, +got)\n%s", d)
					http.Error(w, msg, http.StatusBadRequest)
					t.Error(errors.New(msg))
					return
				}
			} else {
				gotMagic := body[:len(k8sProtoMagic)]
				if !bytes.Equal(gotMagic, k8sProtoMagic) {
					msg := fmt.Sprintf("unexpected magic sequence: %s", gotMagic)
					http.Error(w, msg, http.StatusBadRequest)
					t.Error(errors.New(msg))
					return
				}

				if wantObjMeta != nil {
					if d, err := cmpObjMeta(wantObjMeta, body[len(k8sProtoMagic):]); err != nil {
						t.Fatalf("Failed to compare ObjectMeta: %v", err)
						return
					} else if d != "" {
						msg := fmt.Sprintf("Mismatching `ObjectMeta': (-want, +got)\n%s", d)
						http.Error(w, msg, http.StatusBadRequest)
						t.Error(errors.New(msg))
						return
					}
				}

			}

			s := &metav1.Status{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Status",
					APIVersion: "v1",
				},
				Details: &metav1.StatusDetails{
					Kind:  "pod",
					Group: "",
					Name:  "test",
				},
			}
			bs, _ := apiruntime.Encode(unstructured.UnstructuredJSONScheme, s)
			write(w, bs)
		}
	}))

	return ks.Client(), ks.URL, ks.Close
}

type protoRegistry struct{}

func (*protoRegistry) UnstableProtoMessageType(name string) (reflect.Type, error) {
	if t := proto.MessageType(name); t != nil {
		return t, nil
	}
	if t := gogo_proto.MessageType(name); t != nil {
		return t, nil
	}
	return nil, nil
}

func (*protoRegistry) UnstableEnumValueMap(name string) map[string]int32 {
	if ev := proto.EnumValueMap(name); ev != nil {
		return ev
	}
	if ev := gogo_proto.EnumValueMap(name); ev != nil {
		return ev
	}
	return nil
}

func addImports(t *testing.T, pkgs starlark.StringDict) {
	for val, group := range map[string]string{
		"certificates": "k8s.io.api.certificates.v1",
		"corev1":       "k8s.io.api.core.v1",
		"ext":          "k8s.io.apiextensions_apiserver.pkg.apis.apiextensions.v1beta1",
		"metav1":       "k8s.io.apimachinery.pkg.apis.meta.v1",
		"rbacv1":       "k8s.io.api.rbac.v1",
	} {
		v, _, err := util.Eval(t.Name(), fmt.Sprintf("proto.package(%q)", group), nil, pkgs)
		if err != nil {
			t.Fatal(err)
		}
		pkgs[val] = v
	}
}

var isopodLabels = map[string]string{
	"heritage": "isopod",
}

func withNewLabels(old, add map[string]string) map[string]string {
	new := map[string]string{}
	for k, v := range old {
		new[k] = v
	}
	for k, v := range add {
		new[k] = v
	}
	return new
}

const testPodYaml = `
apiVersion: v1
kind: Pod
metadata:
  name: nginx
  namespace: default
spec:
  containers:
  - name: nginx
    image: nginx:latest
`

const testNSYaml = `
apiVersion: v1
kind: Namespace
metadata:
  name: istio-system
`

func TestKubePackage(t *testing.T) {
	pkgs := skycfg.UnstablePredeclaredModules(&protoRegistry{})
	addImports(t, pkgs)

	urls := func(url ...string) []string {
		return url
	}
	for _, tc := range []struct {
		name         string
		expr         string
		gotObj       apiruntime.Object
		wantURLs     []string
		wantJSON     bool
		wantPodMeta  *metav1.ObjectMeta
		wantDeletion metav1.DeletionPropagation
		wantErr      string
		wantResult   string
	}{
		{
			name:     "Create Pod",
			expr:     `kube.put(name='foo', namespace='bar', data=[corev1.Pod()])`,
			wantURLs: urls("/api/v1/namespaces/bar/pods"),
			wantPodMeta: &metav1.ObjectMeta{
				Name:        "foo",
				Namespace:   "bar",
				Labels:      isopodLabels,
				Annotations: map[string]string{ctxAnnotationKey: `{"env":"test"}`},
			},
		},
		{
			name: "Update Pod",
			expr: `kube.put(name='foo', namespace='bar', data=[corev1.Pod()])`,
			gotObj: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantURLs: urls("/api/v1/namespaces/bar/pods/foo", "/api/v1/namespaces/bar/pods/foo"),
			wantPodMeta: &metav1.ObjectMeta{
				Name:        "foo",
				Namespace:   "bar",
				Labels:      isopodLabels,
				Annotations: map[string]string{ctxAnnotationKey: `{"env":"test"}`},
			},
		},
		{
			name: "Get Pod",
			expr: `kube.get(pod='bar/foo')`,
			gotObj: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantURLs:   urls("/api/v1/namespaces/bar/pods/foo"),
			wantResult: `<k8s.io.api.core.v1.Pod TypeMeta:<kind:"Pod" apiVersion:"v1" > metadata:<name:"foo" creationTimestamp:<0001-01-01T00:00:00Z> > spec:<> status:<> >`,
		},
		{
			name: "Pod Exists",
			expr: `kube.exists(pod='bar/foo')`,
			gotObj: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantURLs:   urls("/api/v1/namespaces/bar/pods/foo"),
			wantResult: `True`,
		},
		{
			name: "Get Pod in JSON",
			expr: `kube.get(pod='bar/foo', json=True)`,
			gotObj: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantURLs:   urls("/api/v1/namespaces/bar/pods/foo"),
			wantResult: `map["apiVersion":"v1" "kind":"Pod" "metadata":map["creationTimestamp":None "name":"foo"] "spec":map["containers":None] "status":map[]]`,
		},
		{
			name: "Get Pods as list",
			expr: `kube.get(pod="bar/", json=True)`,
			gotObj: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PodList",
					APIVersion: "v1",
				},
				Items: []corev1.Pod{{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Pod",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
				}},
			},
			wantURLs:   urls("/api/v1/namespaces/bar/pods"),
			wantResult: `map["apiVersion":"v1" "items":[map["apiVersion":"v1" "kind":"Pod" "metadata":map["creationTimestamp":None "name":"foo"] "spec":map["containers":None] "status":map[]]] "kind":"PodList" "metadata":map[]]`,
		},
		{
			name: "Get Pods as list with field selector: hit",
			expr: `kube.get(pod="bar/?fieldSelector=metadata.name=foo", json=True)`,
			gotObj: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PodList",
					APIVersion: "v1",
				},
				Items: []corev1.Pod{{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Pod",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
				}},
			},
			wantURLs:   urls("/api/v1/namespaces/bar/pods/"),
			wantResult: `map["apiVersion":"v1" "items":[map["apiVersion":"v1" "kind":"Pod" "metadata":map["creationTimestamp":None "name":"foo"] "spec":map["containers":None] "status":map[]]] "kind":"PodList" "metadata":map[]]`,
		},
		{
			name: "Get Pods as list with field selector: miss",
			expr: `kube.get(pod="bar/?fieldSelector=metadata.name=bar", json=True)`,
			gotObj: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PodList",
					APIVersion: "v1",
				},
				Items: []corev1.Pod{},
			},
			wantURLs:   urls("/api/v1/namespaces/bar/pods/"),
			wantResult: `map["apiVersion":"v1" "items":[] "kind":"PodList" "metadata":map[]]`,
		},
		{
			name: "Update Service",
			expr: `kube.put(name='foo', namespace='bar', data=[corev1.Service()])`,
			gotObj: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: corev1.ServiceSpec{
					HealthCheckNodePort: 42,
				},
			},
			wantURLs: urls("/api/v1/namespaces/bar/services/foo", "/api/v1/namespaces/bar/services/foo"),
			wantPodMeta: &metav1.ObjectMeta{
				Name:        "foo",
				Namespace:   "bar",
				Labels:      isopodLabels,
				Annotations: map[string]string{ctxAnnotationKey: `{"env":"test"}`},
			},
		},
		{
			name: "Update Service (set new healthcheck port)",
			expr: `kube.put(name='foo', namespace='bar', data=[corev1.Service(spec = corev1.ServiceSpec(healthCheckNodePort=41))])`,
			gotObj: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantURLs: urls("/api/v1/namespaces/bar/services/foo", "/api/v1/namespaces/bar/services/foo"),
			wantPodMeta: &metav1.ObjectMeta{
				Name:        "foo",
				Namespace:   "bar",
				Labels:      isopodLabels,
				Annotations: map[string]string{ctxAnnotationKey: `{"env":"test"}`},
			},
		},
		{
			name: "Create Namespace",
			expr: `kube.put(name='foo', data=[corev1.Namespace()])`,
			gotObj: &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantURLs: urls("/api/v1/namespaces/foo", "/api/v1/namespaces/foo"),
		},
		{
			name: "Create Namespace (name mismatch)",
			expr: `kube.put(name='foo', namespace='bar', data=[corev1.Namespace()])`,
			gotObj: &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantURLs: urls("/api/v1/namespaces/foo"),
			wantErr:  "<kube.put>: failed to map resource: specified namespace `bar' doesn't match Namespace name: namespace.v1 `bar/foo'",
		},
		{
			name: "Delete Namespace",
			expr: "kube.delete(namespace='test')",
			gotObj: &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			wantURLs: urls("/api/v1/namespaces/test"),
		},
		{
			name: "Delete Pod",
			expr: "kube.delete(pod='default/test')",
			gotObj: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			wantURLs: urls("/api/v1/namespaces/default/pods/test"),
		},
		{
			name: "Delete Pod (blocking)",
			expr: "kube.delete(pod='default/test', foreground=True)",
			gotObj: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			wantURLs:     urls("/api/v1/namespaces/default/pods/test"),
			wantDeletion: "Foreground",
		},
		{
			name: "Get Deployment",
			expr: "kube.get(deployment='default/test', wait='10s', api_group='apps')",
			gotObj: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			wantURLs:   urls("/apis/apps/v1/namespaces/default/deployments/test"),
			wantResult: `<k8s.io.api.apps.v1.Deployment TypeMeta:<kind:"Deployment" apiVersion:"apps/v1" > metadata:<name:"test" creationTimestamp:<0001-01-01T00:00:00Z> > spec:<template:<metadata:<creationTimestamp:<0001-01-01T00:00:00Z> > spec:<> > strategy:<> > status:<> >`,
		},
		{
			name: "Delete Deployment",
			expr: "kube.delete(deployment='default/test', api_group='apps')",
			gotObj: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			wantURLs: urls("/apis/apps/v1/namespaces/default/deployments/test"),
		},
		{
			name:     "Labels must be appended",
			expr:     `kube.put(name='foo', namespace='bar', data=[corev1.Pod(metadata=metav1.ObjectMeta(labels={'foobar': '42'}, annotations={"snafoo": "42"}))])`,
			wantURLs: urls("/api/v1/namespaces/bar/pods"),
			wantPodMeta: &metav1.ObjectMeta{
				Name:        "foo",
				Namespace:   "bar",
				Labels:      withNewLabels(isopodLabels, map[string]string{"foobar": "42"}),
				Annotations: map[string]string{ctxAnnotationKey: `{"env":"test"}`, "snafoo": "42"},
			},
		},
		{
			name:    "Override Namespace (Failure)",
			expr:    `kube.put(name='test', namespace='default', data=[corev1.Pod(metadata=metav1.ObjectMeta(namespace='foobar'))])`,
			wantErr: "<kube.put>: failed to validate/apply metadata for object 0 => k8s.io.api.core.v1.Pod: namespace=`default' argument does not match object's .metadata.namespace=`foobar'",
		},
		{
			name:     "Create CRD definition",
			expr:     `kube.put(name='foo', api_group='apiextensions.k8s.io', data=[ext.CustomResourceDefinition()])`,
			wantURLs: urls("/apis/apiextensions.k8s.io/v1beta1/customresourcedefinitions"),
		},
		{
			name:     "Create YAML object",
			expr:     fmt.Sprintf(`kube.put_yaml(name='nginx', namespace='default', data=["""%s"""])`, testPodYaml),
			wantURLs: urls("/api/v1/namespaces/default/pods"),
			wantJSON: true,
			wantPodMeta: &metav1.ObjectMeta{
				Name:        "nginx",
				Namespace:   "default",
				Labels:      isopodLabels,
				Annotations: map[string]string{ctxAnnotationKey: `{"env":"test"}`},
			},
		},
		{
			name: "Update YAML object",
			expr: fmt.Sprintf(`kube.put_yaml(name='nginx', namespace='default', data=["""%s"""])`, testPodYaml),
			gotObj: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "nginx",
					Namespace:       "default",
					ResourceVersion: "42",
				},
			},
			wantURLs: urls("/api/v1/namespaces/default/pods/nginx", "/api/v1/namespaces/default/pods/nginx"),
			wantJSON: true,
			wantPodMeta: &metav1.ObjectMeta{
				Name:            "nginx",
				Namespace:       "default",
				Labels:          isopodLabels,
				Annotations:     map[string]string{ctxAnnotationKey: `{"env":"test"}`},
				ResourceVersion: "42",
			},
		},
		{
			name: "Create CSR Subresource",
			expr: `kube.put(name='foo', subresource='approval', api_group='certificates.k8s.io', data=[certificates.CertificateSigningRequest()])`,
			gotObj: &certificatesv1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1",
				},
			},
			wantURLs: []string{
				"/apis/certificates.k8s.io/v1/certificatesigningrequests/foo",
				"/apis/certificates.k8s.io/v1/certificatesigningrequests/foo/approval",
			},
		},
		{
			name: "Create Cluster Scoped object",
			expr: fmt.Sprintf(`kube.put_yaml(name='istio-system', data=["""%s"""])`, testNSYaml),
			wantPodMeta: &metav1.ObjectMeta{
				Name:            "istio-system",
				ResourceVersion: "42",
				Labels:          isopodLabels,
				Annotations:     map[string]string{ctxAnnotationKey: `{"env":"test"}`},
			},
			wantJSON: true,
			gotObj: &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "istio-system",
					ResourceVersion: "42",
				},
			},
			wantURLs: urls("/api/v1/namespaces/istio-system", "/api/v1/namespaces/istio-system"),
		},
	} {
		wantDel := metav1.DeletePropagationBackground
		if tc.wantDeletion != "" {
			wantDel = tc.wantDeletion
		}

		fakeHTTPClient, srvURL, closeFn := fakeKubernetes(t, tc.gotObj, tc.wantURLs, tc.wantJSON, tc.wantPodMeta, wantDel)
		defer closeFn()

		u, err := url.Parse(srvURL)
		if err != nil {
			t.Fatal(err)
		}

		h := "https://" + u.Host
		tlsConfig := rest.TLSClientConfig{
			Insecure: true,
		}
		pkgs["kube"] = &kubePackage{
			dClient:    fakeDiscovery(),
			dynClient:  dynamic.NewForConfigOrDie(&rest.Config{Host: h, TLSClientConfig: tlsConfig}),
			httpClient: fakeHTTPClient,
			Master:     h,
		}

		sCtx := &addon.SkyCtx{Attrs: starlark.StringDict{"env": starlark.String("test")}}
		t.Run(tc.name, func(t *testing.T) {
			v, _, err := util.Eval("kube", tc.expr, sCtx, pkgs)

			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if tc.wantErr != gotErr {
				t.Errorf("Unexpected error.\nWant:\n\t%s\nGot:\n\t%s", tc.wantErr, gotErr)
			}
			gotV := ""
			if v != nil && v.String() != noneValue {
				gotV = v.String()
			}
			if tc.wantResult != gotV {
				t.Fatalf("Unexpected expression result.\nWant: %s\nGot: %s", tc.wantResult, gotV)
			}
		})
	}
}

func TestKubeExists(t *testing.T) {
	pkgs := skycfg.UnstablePredeclaredModules(&protoRegistry{})
	addImports(t, pkgs)

	k, kClose, err := NewFake(false)
	if err != nil {
		t.Error(err)
	}
	defer kClose()

	pkgs["kube"] = k

	for _, tc := range []struct {
		name       string
		expr       string
		wantErr    string
		wantResult string
	}{
		{
			name:       "Pod doesn't exist",
			expr:       `kube.exists(pod='bar/foo')`,
			wantResult: `False`,
		},
	} {
		sCtx := &addon.SkyCtx{Attrs: starlark.StringDict{"env": starlark.String("test")}}
		t.Run(tc.name, func(t *testing.T) {
			v, _, err := util.Eval("kube", tc.expr, sCtx, pkgs)

			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if tc.wantErr != gotErr {
				t.Errorf("Unexpected error.\nWant:\n\t%s\nGot:\n\t%s", tc.wantErr, gotErr)
			}
			gotV := ""
			if v != nil && v.String() != noneValue {
				gotV = v.String()
			}
			if tc.wantResult != gotV {
				t.Fatalf("Unexpected expression result.\nWant: %s\nGot: %s", tc.wantResult, gotV)
			}
		})
	}
}

func TestErrImmutableRessource(t *testing.T) {
	got := ErrImmutableRessource("roleRef", &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
	})
	want := errors.New("failed to update roleRef of resource rbac.authorization.k8s.io/v1, Kind=ClusterRoleBinding: cannot update immutable. Use -force to delete and recreate")
	if want.Error() != got.Error() {
		t.Errorf("Unexpected error.\nWant:\n\t%s\nGot:\n\t%s", want, got)
	}
}

func TestUpdateImmutableResources(t *testing.T) {
	pkgs := skycfg.UnstablePredeclaredModules(&protoRegistry{})
	addImports(t, pkgs)

	for _, tc := range []struct {
		name         string
		exprCreate   string
		exprUpdate   string
		forceEnabled bool
		wantErr      string
		wantResult   string
	}{
		{
			name:       "Update ClusterRoleBinding",
			exprCreate: `kube.put(name='foo', namespace='bar', api_group='rbac.authorization.k8s.io', data=[rbacv1.ClusterRoleBinding(roleRef=rbacv1.RoleRef(name="foo",kind="ClusterRole"))])`,
			exprUpdate: `kube.put(name='foo', namespace='bar', api_group='rbac.authorization.k8s.io', data=[rbacv1.ClusterRoleBinding(roleRef=rbacv1.RoleRef(name="bar",kind="ClusterRole"))])`,
			wantErr:    fmt.Sprintf("<kube.put>: %s", ErrImmutableRessource("roleRef", &corev1.ObjectReference{})),
		},
		{
			name:         "Update ClusterRoleBinding force",
			exprCreate:   `kube.put(name='foo', namespace='bar', api_group='rbac.authorization.k8s.io', data=[rbacv1.ClusterRoleBinding(roleRef=rbacv1.RoleRef(name="foo",kind="ClusterRole"))])`,
			exprUpdate:   `kube.put(name='foo', namespace='bar', api_group='rbac.authorization.k8s.io', data=[rbacv1.ClusterRoleBinding(roleRef=rbacv1.RoleRef(name="bar",kind="ClusterRole"))])`,
			forceEnabled: true,
		},
		{
			name:       "Update ClusterRoleBinding",
			exprCreate: `kube.put(name='foo', namespace='bar', data=[corev1.Service(spec = corev1.ServiceSpec(healthCheckNodePort=41))])`,
			exprUpdate: `kube.put(name='foo', namespace='bar', data=[corev1.Service(spec = corev1.ServiceSpec(healthCheckNodePort=42))])`,
			wantErr:    fmt.Sprintf("<kube.put>: %s", ErrImmutableRessource(".spec.healthCheckNodePort", &corev1.ObjectReference{})),
		},
		{
			name:         "Update ClusterRoleBinding force",
			exprCreate:   `kube.put(name='foo', namespace='bar', data=[corev1.Service(spec = corev1.ServiceSpec(healthCheckNodePort=41))])`,
			exprUpdate:   `kube.put(name='foo', namespace='bar', data=[corev1.Service(spec = corev1.ServiceSpec(healthCheckNodePort=42))])`,
			forceEnabled: true,
		},
	} {
		sCtx := &addon.SkyCtx{Attrs: starlark.StringDict{"env": starlark.String("test")}}
		t.Run(tc.name, func(t *testing.T) {

			k, kClose, err := NewFake(tc.forceEnabled)
			if err != nil {
				t.Error(err)
			}
			defer kClose()

			pkgs["kube"] = k

			_, _, err = util.Eval("kube", tc.exprCreate, sCtx, pkgs)
			if err != nil {
				t.Error(err)
			}
			v, _, err := util.Eval("kube", tc.exprUpdate, sCtx, pkgs)

			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if tc.wantErr != gotErr {
				t.Errorf("Unexpected error.\nWant:\n\t%s\nGot:\n\t%s", tc.wantErr, gotErr)
			}
			gotV := ""
			if v != nil && v.String() != noneValue {
				gotV = v.String()
			}
			if tc.wantResult != gotV {
				t.Fatalf("Unexpected expression result.\nWant: %s\nGot: %s", tc.wantResult, gotV)
			}
		})
	}
}
