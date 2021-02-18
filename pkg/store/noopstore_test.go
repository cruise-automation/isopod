// Copyright 2021 GM Cruise LLC
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

package store

import (
	"testing"

	// Required to add flags for tests to run properly.
	_ "github.com/golang/glog"
)

func checkErr(t *testing.T, err error, funcName string) {
	if err != nil {
		t.Errorf("%s returned a non-nil error.", funcName)
	}
}

// TestNoopStore tests all methods in the Store interface for the NoopStore.
// Since they are all noops, the testing is pretty simple.
func TestNoopStore(t *testing.T) {
	store := NoopStore{}

	rollout, err := store.CreateRollout()
	if rollout == nil {
		t.Errorf("CreateRollout returned nil instead of empty rollout.")
	}
	checkErr(t, err, "CreateRollout")

	runID, err := store.PutAddonRun("", nil)
	if runID != "" {
		t.Errorf("PutAddonRun returned a non-empty string")
	}
	checkErr(t, err, "PutAddonRun")

	err = store.CompleteRollout("")
	checkErr(t, err, "CompleteRollout")

	_, found, err := store.GetLive()
	if found {
		t.Errorf("GetLive returned true for `found`. It should not find anything.")
	}
	checkErr(t, err, "GetLive")

	_, found, err = store.GetRollout("")
	if found {
		t.Errorf("GetRollout returned true for `found`. It should not find anything.")
	}
	checkErr(t, err, "GetRollout")
}
