/*
   Copyright 2018 GM Cruise LLC

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package v1alpha

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RBACSyncConfig configures the behavior of the rbac sync process.
//
// RBACSyncConfig should reference only RoleBindings in group specs. In group
// specs that don't use RoleRefs, such as those referencing Google groups, only
// RoleBindings will be created.
//
// The RoleBindings created by this configuration will be bound to the same
// namespace as the configuration.
type RBACSyncConfig struct {
	// +optional
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec   Spec   `json:"spec"`
	Status Status `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RBACSyncConfigList is a list of RBACSyncConfigs
type RBACSyncConfigList struct {
	// +optional
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ListMeta `json:"metadata"`

	Items []RBACSyncConfig `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterRBACSyncConfig configures the behavior of the rbac sync process.
//
// All bindings created from this object will be ClusterRoleBindings that
// reference cluster-wide roles.
//
// This type is nearly identical to RBACSyncConfig, except it is not namespaced.
type ClusterRBACSyncConfig struct {
	// +optional
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec   Spec   `json:"spec"`
	Status Status `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterRBACSyncConfigList is a list of ClusterRBACSyncConfigs
type ClusterRBACSyncConfigList struct {
	// +optional
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ListMeta `json:"metadata"`

	Items []ClusterRBACSyncConfig `json:"items"`
}

type Spec struct {
	// Bindings declare a group name and a role ref. Each binding declared here
	// will result in the creation of a RoleBinding or ClusterRoleBinding,
	// depending on whether this spec is namespaced or cluster scoped.
	//
	// Groups referenced here may be part of an upstream or defined in
	// memberships. If they are available in both, the subjects will be merged.
	Bindings []Binding `json:"bindings,omitempty"`

	// Memberships provides a set groups that are statically configured as part
	// this config.
	//
	// Subjects referenced here may be declared to add supplemental members of
	// upstream groups or to declare groups that aren't part of the upstream.
	//
	// If these overlap with an upstream group definition, such as in gsuite,
	// the members will be merged.
	Memberships []Membership `json:"memberships,omitempty"`
}

type Status struct {
	// TODO(sday): Output status information from sync.
}

// Binding is the central definition for the RBACSyncConfig and
// ClusterRBACSyncConfig. It maps a group to a RoleRef. The RoleRef will be
// combined with the subjects resolved in the group to assemble the RoleBinding
// or ClusterRoleBinding.
type Binding struct {
	Group   string         `json:"group,omitempty"`
	RoleRef rbacv1.RoleRef `json:"roleRef,omitempty"`
}

// Membership describes a group and its set of members.
//
// Memberships can be used to declare groups that are not part of the upstream
// or to add supplemental members to an upstream group.
type Membership struct {
	Group    string           `json:"group,omitempty"`
	Subjects []rbacv1.Subject `json:"subjects,omitempty"`
}
