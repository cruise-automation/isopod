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
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/cruise-automation/isopod/pkg/store"
)

const addonText = `
def install(ctx):
  pass

def status(ctx):
  pass

def remove(ctx):
  pass
`

func waitN(t *testing.T, c chan *v1.ConfigMap, n int) {
	for i := 0; i < n; i++ {
		select {
		case cm := <-c:
			t.Logf("Got configmap from channel: %s/%s", cm.Namespace, cm.Name)
		case <-time.After(5 * time.Second):
			t.Error("Informer did not get the added configmap")
			return
		}
	}
}

func TestRollout(t *testing.T) {
	ctx := context.Background()

	// Create the fake client.
	client := fake.NewSimpleClientset()

	// We will create an informer that writes added pods to a channel.
	ch := make(chan *v1.ConfigMap, 1)
	informer := informers.NewSharedInformerFactory(client, 0)
	cmInformer := informer.Core().V1().ConfigMaps().Informer()
	cmInformer.AddEventHandler(&cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cm := obj.(*v1.ConfigMap)
			t.Logf("configmap added: %s/%s", cm.Namespace, cm.Name)
			ch <- cm
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldCM := oldObj.(*v1.ConfigMap)
			newCM := newObj.(*v1.ConfigMap)
			t.Logf("replacing configmap `%s/%s' with: %s/%s", oldCM.Namespace, oldCM.Name, newCM.Namespace, newCM.Name)
			ch <- newCM
		},
	})

	// Make sure informers are running.
	informer.Start(ctx.Done())

	// This is not required in tests, but it serves as a proof-of-concept by
	// ensuring that the informer goroutine have warmed up and called List before
	// we send any events to it.
	for !cmInformer.HasSynced() {
		time.Sleep(10 * time.Millisecond)
	}

	ks := &Store{clientset: client, namespace: "test-ns"}

	r, err := ks.CreateRollout()
	if err != nil {
		t.Errorf("error creating rollout: %v", err)
	}
	waitN(t, ch, 1)

	_, err = ks.PutAddonRun(r.ID, &store.AddonRun{Name: "test-addon", Modules: map[string]string{"main.ipd": addonText}})
	if err != nil {
		t.Errorf("error creating run for rollout `%s': %v", r.ID, err)
	}
	waitN(t, ch, 2)

	if err = ks.CompleteRollout(r.ID); err != nil {
		t.Errorf("error completing rollout `%s': %v", r.ID, err)
	}
	waitN(t, ch, 1)
}
