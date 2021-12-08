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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"reflect"
	"strings"

	log "github.com/golang/glog"
	"go.starlark.net/starlark"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	coretesting "k8s.io/client-go/testing"

	rbacsyncv1alpha "github.com/cruise-automation/rbacsync/pkg/apis/rbacsync/v1alpha"
	arkv1 "github.com/heptio/ark/pkg/apis/ark/v1"
	istio "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiov1b1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	istiosecurityv1beta1 "istio.io/client-go/pkg/apis/security/v1beta1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	admissionregistrationv1b1 "k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	appsv1beta1 "k8s.io/api/apps/v1beta1"
	appsv1beta2 "k8s.io/api/apps/v1beta2"
	authenticationv1 "k8s.io/api/authentication/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	batchv1 "k8s.io/api/batch/v1"
	batchv1b1 "k8s.io/api/batch/v1beta1"
	csr "k8s.io/api/certificates/v1"
	csrv1b1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1beta1 "k8s.io/api/scheduling/v1beta1"
	storagev1 "k8s.io/api/storage/v1"
	storagev1beta1 "k8s.io/api/storage/v1beta1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	vpav1beta2 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1beta2"
	apiregistrationv1b1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1beta1"

	isopod "github.com/cruise-automation/isopod/pkg"
)

type fakeKube struct {
	m map[string][]byte
}

func nameFromObj(obj apiruntime.Object) (string, error) {
	metaVal := reflect.ValueOf(obj).Elem().FieldByName("ObjectMeta")
	if !metaVal.IsValid() {
		return "", errors.New("could not extract .ObjectMeta")
	}
	nameVal := metaVal.FieldByName("Name")
	if !nameVal.IsValid() {
		return "", errors.New("could not extract .ObjectMeta.Name")
	}
	return nameVal.String(), nil
}

func write(w io.Writer, bs []byte) {
	if _, err := w.Write(bs); err != nil {
		log.Errorf("failed to write `%v' to response: %v", bs, err)
	}
}

func (h *fakeKube) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		obj, _, err := decodeFn(data, nil, nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to deserialize: %v", err), http.StatusBadRequest)
			return
		}

		name, err := nameFromObj(obj)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		h.m[path.Join(r.URL.Path, name)] = data

	case http.MethodPut:
		// If it's a CSR subresource approval request, ensure that the CSR resource exists already.
		if strings.HasSuffix(r.URL.Path, "/approval") {
			_, ok := h.m[strings.TrimSuffix(r.URL.Path, "/approval")]
			if !ok {
				http.Error(w, "not found", http.StatusMethodNotAllowed)
				return
			}
		} else {
			// Check for the existence of any other resources.
			_, ok := h.m[r.URL.Path]
			if !ok {
				http.Error(w, "not found", http.StatusMethodNotAllowed)
				return
			}
		}

		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// If it's a CSR approval request, inject some fake certificate contents into the CSR.
		if strings.HasSuffix(r.URL.Path, "/approval") {
			data := h.m[strings.TrimSuffix(r.URL.Path, "/approval")]
			obj, _, err := decodeFn(data, nil, nil)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			csReq, ok := obj.(*csr.CertificateSigningRequest)
			if !ok {
				csReq, okV1b1 := obj.(*csrv1b1.CertificateSigningRequest)
				if !okV1b1 {
					http.Error(w, "obj is not a *csr.CertificateSigningRequest", http.StatusBadRequest)
					return
				}
				csReq.TypeMeta = metav1.TypeMeta{
					APIVersion: "certificates.k8s.io/v1beta1",
					Kind:       "CertificateSigningRequest",
				}
				csReq.Status.Certificate = []byte("cert")
				data, err = apiruntime.Encode(unstructured.UnstructuredJSONScheme, csReq)
			} else {
				csReq.TypeMeta = metav1.TypeMeta{
					APIVersion: "certificates.k8s.io/v1",
					Kind:       "CertificateSigningRequest",
				}
				csReq.Status.Certificate = []byte("cert")
				data, err = apiruntime.Encode(unstructured.UnstructuredJSONScheme, csReq)
			}

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			h.m[strings.TrimSuffix(r.URL.Path, "/approval")] = data
		}

		h.m[r.URL.Path] = data

	case http.MethodGet:
		res, ok := h.m[r.URL.Path]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		write(w, res)
		return

	case http.MethodDelete:
		res, ok := h.m[r.URL.Path]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		obj, gvk, err := decodeFn(res, nil, nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to deserialize: %v", err), http.StatusBadRequest)
			return
		}

		name, err := nameFromObj(obj)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s := &metav1.Status{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Status",
				APIVersion: "v1",
			},
			Details: &metav1.StatusDetails{
				Kind:  gvk.Kind,
				Group: gvk.GroupVersion().String(),
				Name:  name,
			},
		}
		bs, _ := apiruntime.Encode(unstructured.UnstructuredJSONScheme, s)
		write(w, bs)
		return
	default:
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
	}

	s := &metav1.Status{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Status",
			APIVersion: "v1",
		},
		Details: &metav1.StatusDetails{},
	}
	bs, _ := apiruntime.Encode(unstructured.UnstructuredJSONScheme, s)
	write(w, bs)
}

func newFakeModule(k *kubePackage) *isopod.Module {
	return &isopod.Module{
		Name: "kube",
		Attrs: starlark.StringDict{
			kubePutMethod:              starlark.NewBuiltin("kube."+kubePutMethod, k.kubePutFn),
			kubeDeleteMethod:           starlark.NewBuiltin("kube."+kubeDeleteMethod, k.kubeDeleteFn),
			kubeResourceQuantityMethod: starlark.NewBuiltin("kube."+kubeResourceQuantityMethod, resourceQuantityFn),
			kubePutYamlMethod:          starlark.NewBuiltin("kube."+kubePutYamlMethod, k.kubePutYamlFn),
			kubeGetMethod:              starlark.NewBuiltin("kube."+kubeGetMethod, k.kubeGetFn),
			kubeExistsMethod:           starlark.NewBuiltin("kube."+kubeExistsMethod, k.kubeExistsFn),
			kubeFromIntMethod:          starlark.NewBuiltin("kube."+kubeFromIntMethod, fromIntFn),
			kubeFromStrMethod:          starlark.NewBuiltin("kube."+kubeFromStrMethod, fromStringFn),
		},
	}
}

// fakeDiscovery return fake discovery client that supports
// pods API resource.
func fakeDiscovery() discovery.DiscoveryInterface {
	fake := &fakediscovery.FakeDiscovery{Fake: &coretesting.Fake{}}
	apps := []metav1.APIResource{
		{Name: "deployments", Namespaced: true, Kind: "Deployment"},
		{Name: "controllerrevisions", Namespaced: true, Kind: "ControllerRevision"},
		{Name: "daemonsets", Namespaced: true, Kind: "DaemonSet"},
		{Name: "replicasets", Namespaced: true, Kind: "ReplicaSet"},
		{Name: "statefulsets", Namespaced: true, Kind: "StatefulSet"},
	}
	fake.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: corev1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "bindings", Namespaced: true, Kind: "Binding"},
				{Name: "componentstatuses", Kind: "ComponentStatus"},
				{Name: "configmaps", Namespaced: true, Kind: "ConfigMap"},
				{Name: "endpoints", Namespaced: true, Kind: "Endpoints"},
				{Name: "events", Namespaced: true, Kind: "Event"},
				{Name: "limitranges", Namespaced: true, Kind: "LimitRange"},
				{Name: "namespaces", Kind: "Namespace"},
				{Name: "nodes", Kind: "Node"},
				{Name: "persistentvolumeclaims", Namespaced: true, Kind: "PersistentVolumeClaim"},
				{Name: "persistentvolumes", Kind: "PersistentVolume"},
				{Name: "pods", Namespaced: true, Kind: "Pod"},
				{Name: "podtemplates", Namespaced: true, Kind: "PodTemplate"},
				{Name: "replicationcontrollers", Namespaced: true, Kind: "ReplicationController"},
				{Name: "resourcequotas", Namespaced: true, Kind: "ResourceQuota"},
				{Name: "secrets", Namespaced: true, Kind: "Secret"},
				{Name: "serviceaccounts", Namespaced: true, Kind: "ServiceAccount"},
				{Name: "services", Namespaced: true, Kind: "Service"},
			},
		},
		{
			GroupVersion: rbacv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "clusterroles", Kind: "ClusterRole"},
				{Name: "clusterrolebindings", Kind: "ClusterRoleBinding"},
				{Name: "clusterroles", Kind: "ClusterRole"},
				{Name: "rolebindings", Namespaced: true, Kind: "RoleBinding"},
				{Name: "roles", Namespaced: true, Kind: "Role"},
			},
		},
		{
			GroupVersion: appsv1.SchemeGroupVersion.String(),
			APIResources: apps,
		},
		{
			GroupVersion: appsv1beta1.SchemeGroupVersion.String(),
			APIResources: apps,
		},
		{
			GroupVersion: appsv1beta2.SchemeGroupVersion.String(),
			APIResources: apps,
		},
		{
			GroupVersion: apiextensionsv1beta1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "customresourcedefinitions", Kind: "CustomResourceDefinition"},
			},
		},
		{
			GroupVersion: apiextensionsv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "customresourcedefinitions", Kind: "CustomResourceDefinition"},
			},
		},
		{
			GroupVersion: storagev1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "storageclasses", Kind: "StorageClass"},
				{Name: "volumeattachments", Kind: "VolumeAttachment"},
			},
		},
		{
			GroupVersion: storagev1beta1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "storageclasses", Kind: "StorageClass"},
				{Name: "volumeattachments", Kind: "VolumeAttachment"},
			},
		},
		{
			GroupVersion: extensionsv1beta1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "ingresses", Namespaced: true, Kind: "Ingress"},
				{Name: "networkpolicies", Namespaced: true, Kind: "NetworkPolicy"},
				{Name: "podsecuritypolicies", Kind: "PodSecurityPolicy"},
			},
		},
		{
			GroupVersion: networkingv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "networkpolicies", Namespaced: true, Kind: "NetworkPolicy"},
			},
		},
		{
			GroupVersion: authenticationv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "tokenreviews", Kind: "TokenReview"},
			},
		},
		{
			GroupVersion: autoscalingv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "horizontalpodautoscalers", Kind: "HorizontalPodAutoscaler"},
			},
		},
		{
			GroupVersion: autoscalingv2beta1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "horizontalpodautoscalers", Kind: "HorizontalPodAutoscaler"},
			},
		},
		{
			GroupVersion: autoscalingv2beta2.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "horizontalpodautoscalers", Kind: "HorizontalPodAutoscaler"},
			},
		},
		{
			GroupVersion: policyv1beta1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "poddisruptionbudgets", Namespaced: true, Kind: "PodDisruptionBudget"},
				{Name: "podsecuritypolicies", Kind: "PodSecurityPolicy"},
			},
		},
		{
			GroupVersion: rbacsyncv1alpha.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "clusterrbacsyncconfigs", Kind: "ClusterRBACSyncConfig"},
				{Name: "rbacsyncconfigs", Namespaced: true, Kind: "RBACSyncConfig"},
			},
		},
		{
			GroupVersion: batchv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "jobs", Namespaced: true, Kind: "Job"},
			},
		},
		{
			GroupVersion: batchv1b1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "cronjobs", Namespaced: true, Kind: "CronJob"},
			},
		},
		{
			GroupVersion: arkv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "backups", Namespaced: true, Kind: "Backup"},
				{Name: "backupstoragelocations", Namespaced: true, Kind: "BackupStorageLocation"},
				{Name: "configs", Namespaced: true, Kind: "Config"},
				{Name: "deletebackuprequests", Namespaced: true, Kind: "DeleteBackupRequest"},
				{Name: "downloadrequests", Namespaced: true, Kind: "DownloadRequest"},
				{Name: "podvolumebackups", Namespaced: true, Kind: "PodVolumeBackup"},
				{Name: "podvolumerestores", Namespaced: true, Kind: "PodVolumeRestore"},
				{Name: "resticrepositories", Namespaced: true, Kind: "ResticRepository"},
				{Name: "restores", Namespaced: true, Kind: "Restore"},
				{Name: "schedules", Namespaced: true, Kind: "Schedule"},
			},
		},
		{
			GroupVersion: istio.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "sidecar", Namespaced: true, Kind: "Sidecar"},
				{Name: "virtualservice", Namespaced: true, Kind: "VirtualService"},
				{Name: "destinationrule", Namespaced: true, Kind: "DestinationRule"},
				{Name: "gateway", Namespaced: true, Kind: "Gateway"},
				{Name: "serviceentry", Kind: "ServiceEntry"},
				{Name: "envoyfilter", Namespaced: true, Kind: "EnvoyFilter"},
			},
		},
		{
			GroupVersion: istiov1b1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "sidecar", Namespaced: true, Kind: "Sidecar"},
				{Name: "virtualservice", Namespaced: true, Kind: "VirtualService"},
				{Name: "destinationrule", Namespaced: true, Kind: "DestinationRule"},
				{Name: "gateway", Namespaced: true, Kind: "Gateway"},
				{Name: "serviceentry", Kind: "ServiceEntry"},
				{Name: "envoyfilter", Namespaced: true, Kind: "EnvoyFilter"},
			},
		},
		{
			GroupVersion: istiosecurityv1beta1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "authorizationpolicy", Namespaced: true, Kind: "AuthorizationPolicy"},
			},
		},
		{
			GroupVersion: csr.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "certificatesigningrequests", Kind: "CertificateSigningRequest"},
			},
		},
		{
			GroupVersion: csrv1b1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "certificatesigningrequests", Kind: "CertificateSigningRequest"},
			},
		},
		{
			GroupVersion: admissionregistrationv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "validatingwebhookconfigurations", Kind: "ValidatingWebhookConfiguration"},
				{Name: "mutatingwebhookconfigurations", Kind: "MutatingWebhookConfiguration"},
			},
		},
		{
			GroupVersion: admissionregistrationv1b1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "validatingwebhookconfigurations", Kind: "ValidatingWebhookConfiguration"},
				{Name: "mutatingwebhookconfigurations", Kind: "MutatingWebhookConfiguration"},
			},
		},
		{
			GroupVersion: schedulingv1beta1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "priorityclass", Kind: "PriorityClass"},
			},
		},
		{
			GroupVersion: apiregistrationv1b1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "apiservice", Kind: "APIService"},
			},
		},
		{
			GroupVersion: vpav1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "verticalpodautoscalers", Kind: "VerticalPodAutoscaler"},
			},
		},
		{
			GroupVersion: vpav1beta2.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "verticalpodautoscalers", Kind: "VerticalPodAutoscaler"},
			},
		},
	}
	return fake
}

// NewFake returns a new fake kube module for testing.
// It takes a bool attribute to determine if the starkalrk.HasAttrs object should forcefully update resources
func NewFake(force bool) (m starlark.HasAttrs, closeFn func(), err error) {
	// Create a fake API store with some endpoints pre-populated
	cm := corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		Data: map[string]string{
			"client-ca-file": "contents",
		},
	}
	cmData, err := apiruntime.Encode(unstructured.UnstructuredJSONScheme, &cm)
	if err != nil {
		return nil, nil, err
	}
	fm := map[string][]byte{
		"/api/v1/namespaces/kube-system/configmaps/extension-apiserver-authentication": cmData,
	}

	s := httptest.NewTLSServer(&fakeKube{m: fm})

	u, err := url.Parse(s.URL)
	if err != nil {
		return nil, nil, err
	}

	h := "https://" + u.Host
	tlsConfig := rest.TLSClientConfig{
		Insecure: true,
	}
	rConf := &rest.Config{Host: h, TLSClientConfig: tlsConfig}

	t, err := rest.TransportFor(rConf)
	if err != nil {
		return nil, nil, err
	}

	k := New(
		h,
		fakeDiscovery(),
		dynamic.NewForConfigOrDie(rConf),
		&http.Client{Transport: t},
		false, /* dryRun */
		force,
		false, /* diff */
		nil,   /* diffFilters */
	)

	return newFakeModule(k.(*kubePackage)), s.Close, nil
}
