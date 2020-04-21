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

// Package kube implement Kubernetes storage for rollouts.
package kube

import (
	"context"
	"errors"
	"fmt"

	log "github.com/golang/glog"
	"github.com/rs/xid"
	yaml "gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	"github.com/cruise-automation/isopod/pkg/store"
)

type Store struct {
	namespace string
	clientset kubernetes.Interface
}

// New returns new Kubernetes-based Store implementation.
func New(c kubernetes.Interface, namespace string) *Store {
	return &Store{
		clientset: c,
		namespace: namespace,
	}
}

// CreateRollout implements store.Store.CreateRollout.
func (s *Store) CreateRollout() (*store.Rollout, error) {
	cm, err := s.clientset.CoreV1().ConfigMaps(s.namespace).Create(
		context.TODO(),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rollout-" + xid.New().String(),
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, err
	}
	return &store.Rollout{
		ID: store.RolloutID(cm.Name),
	}, nil
}

// PutAddonRun implements store.Store.PutAddonRun.
func (s *Store) PutAddonRun(id store.RolloutID, addon *store.AddonRun) (store.RunID, error) {
	rollout, err := s.clientset.CoreV1().ConfigMaps(s.namespace).Get(
		context.TODO(),
		string(id),
		metav1.GetOptions{},
	)
	if err != nil {
		return "", err
	}

	mods, err := yaml.Marshal(addon.Modules)
	if err != nil {
		return "", fmt.Errorf("could not marshal addon modules: %v", err)
	}

	ref := metav1.NewControllerRef(rollout, schema.GroupVersionKind{
		Version: "v1",
		Kind:    "ConfigMap",
	})
	runLabels := map[string]string{
		"addon": addon.Name,
		"owner": string(id),
	}
	run, err := s.clientset.CoreV1().ConfigMaps(s.namespace).Create(
		context.TODO(),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("%s-run-%v", addon.Name, xid.New()),
				OwnerReferences: []metav1.OwnerReference{*ref},
				Labels:          runLabels,
			},
			Data: map[string]string{
				"addon":   addon.Name,
				"modules": string(mods),
			},
			BinaryData: addon.Data,
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return "", err
	}

	if rollout.Data == nil {
		rollout.Data = make(map[string]string)
	}
	if e := rollout.Data[addon.Name]; e != "" {
		return "", fmt.Errorf("addon run for addon `%s' already exists: %s", addon.Name, run.Name)
	}
	rollout.Data[addon.Name] = run.Name
	// Assume we are the only Isopod in the cluster and just error-out if
	// something funky is going on (like update race condition) to let operator
	// deal with it.
	_, err = s.clientset.CoreV1().ConfigMaps(s.namespace).Update(
		context.TODO(),
		rollout,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return "", err
	}
	return store.RunID(run.Name), nil
}

// CompleteRollout implements store.Store.CompleteRollout.
func (s *Store) CompleteRollout(id store.RolloutID) error {
	lst, err := s.clientset.CoreV1().ConfigMaps(s.namespace).List(
		context.TODO(),
		metav1.ListOptions{
			FieldSelector: "metadata.name=rollout-live",
			LabelSelector: "rollout=live", // fakeclient is kind of trash and doesn't support field selectors.
			Limit:         1,
		},
	)
	if err != nil {
		return err
	}

	var live *corev1.ConfigMap
	if len(lst.Items) == 0 {
		log.Infof("Creating new live rollout config for `%v'", id)
		_, err = s.clientset.CoreV1().ConfigMaps(s.namespace).Create(
			context.TODO(),
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "rollout-live",
					Labels: map[string]string{"rollout": "live"},
				},
				Data: map[string]string{"rollout": string(id)},
			},
			metav1.CreateOptions{},
		)
	} else {
		live = &lst.Items[0]
		log.Infof("Replacing live rollout config `%s' with: %v", live.Data["rollout"], id)
		live.Data["rollout"] = string(id)
		_, err = s.clientset.CoreV1().ConfigMaps(s.namespace).Update(context.TODO(), live, metav1.UpdateOptions{})
	}

	return err
}

// GetLive implements store.Store.GetLive.
func (s *Store) GetLive() (r *store.Rollout, found bool, err error) {
	return nil, false, errors.New("not implemented")
}

// GetRollout implements store.Store.GetRollout.
func (s *Store) GetRollout(id store.RolloutID) (r *store.Rollout, found bool, err error) {
	return nil, false, errors.New("not implemented")
}
