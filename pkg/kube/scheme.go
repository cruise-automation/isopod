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
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	appsv1beta1 "k8s.io/api/apps/v1beta1"
	appsv1beta2 "k8s.io/api/apps/v1beta2"
	authenticationv1 "k8s.io/api/authentication/v1"
	authenticationv1beta1 "k8s.io/api/authentication/v1beta1"
	authorizationv1 "k8s.io/api/authorization/v1"
	authorizationv1beta1 "k8s.io/api/authorization/v1beta1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	certificatesv1 "k8s.io/api/certificates/v1"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	coordinationv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	rbacv1alpha1 "k8s.io/api/rbac/v1alpha1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	schedulingv1alpha1 "k8s.io/api/scheduling/v1alpha1"
	schedulingv1beta1 "k8s.io/api/scheduling/v1beta1"
	storagev1 "k8s.io/api/storage/v1"
	storagev1alpha1 "k8s.io/api/storage/v1alpha1"
	storagev1beta1 "k8s.io/api/storage/v1beta1"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	vpav1beta2 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1beta2"

	rbacsyncv1alpha "github.com/cruise-automation/rbacsync/pkg/apis/rbacsync/v1alpha"
	arkv1 "github.com/heptio/ark/pkg/apis/ark/v1"
	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	istiosecurityv1beta1 "istio.io/client-go/pkg/apis/security/v1beta1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	apiregistrationv1beta1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1beta1"

	// Import serializer extensions.
	_ "k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
)

var (
	Scheme             = runtime.NewScheme()
	Codecs             = serializer.NewCodecFactory(Scheme)
	localSchemeBuilder = runtime.SchemeBuilder{
		apiextensionsv1beta1.AddToScheme,
		apiextensionsv1.AddToScheme,
		admissionregistrationv1.AddToScheme,
		admissionregistrationv1beta1.AddToScheme,
		appsv1.AddToScheme,
		appsv1beta1.AddToScheme,
		appsv1beta2.AddToScheme,
		arkv1.AddToScheme,
		authenticationv1.AddToScheme,
		authenticationv1beta1.AddToScheme,
		authorizationv1.AddToScheme,
		authorizationv1beta1.AddToScheme,
		autoscalingv1.AddToScheme,
		autoscalingv2beta1.AddToScheme,
		autoscalingv2beta2.AddToScheme,
		batchv1.AddToScheme,
		batchv1beta1.AddToScheme,
		certificatesv1.AddToScheme,
		certificatesv1beta1.AddToScheme,
		coordinationv1beta1.AddToScheme,
		corev1.AddToScheme,
		eventsv1beta1.AddToScheme,
		extensionsv1beta1.AddToScheme,
		networkingv1.AddToScheme,
		policyv1beta1.AddToScheme,
		rbacv1.AddToScheme,
		rbacv1beta1.AddToScheme,
		rbacv1alpha1.AddToScheme,
		rbacsyncv1alpha.AddToScheme,
		schedulingv1alpha1.AddToScheme,
		schedulingv1beta1.AddToScheme,
		storagev1beta1.AddToScheme,
		storagev1.AddToScheme,
		storagev1alpha1.AddToScheme,
		vpav1beta2.AddToScheme,
		vpav1.AddToScheme,
		apiregistrationv1beta1.AddToScheme,
		istionetworkingv1alpha3.AddToScheme,
		istionetworkingv1beta1.AddToScheme,
		istiosecurityv1beta1.AddToScheme,
	}
)

func init() {
	utilruntime.Must(localSchemeBuilder.AddToScheme(Scheme))
}
