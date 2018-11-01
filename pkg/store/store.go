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

// Package store implements persistent store for recording current/past states
// of the addon rollouts.
package store

// RunID is id of an addon run.
type RunID string

// AddonRun represents stored state of an addon run.
type AddonRun struct {
	// Name is a name of an addon associated with the run.
	Name string
	// Modules is map of all modules (each represents a single file)
	// required to run an addon.
	Modules map[string]string

	// Data is opaque data passed in by addon during execution.
	Data map[string][]byte

	// ObjRefs is a slice of object references (could be external to
	// Kubernetes objects) that were part of this run.
	// TODO(dmitry-ilyevskiy): Make this into proper interface definition
	// once the scope of operations is a little bit more defined.
	ObjRefs []interface{}
}

// RolloutID is a unique rollout ID string.
type RolloutID string

// Rollout represents a single rollout - a set of addon runs.
// The last rollout to complete successfully is marked as "live" (there must
// be at most one "live" rollout).
type Rollout struct {
	ID     RolloutID
	Addons []*AddonRun
	Live   bool
}

// Store defines a rollout store interface.
type Store interface {
	// CreateRollout initializes and returns a new *Rollout object with
	// defaults and new RolloutID (committed to the store).
	CreateRollout() (*Rollout, error)

	// PutAddonRun records addon rollout for run id.
	PutAddonRun(id RolloutID, addon *AddonRun) (RunID, error)

	// CompleteRollout marks rollout run as complete (sets it as "live").
	// All further PutAddonRun operations will fail.
	CompleteRollout(id RolloutID) error

	// GetLive returns a single "live" rollout, if found.
	GetLive() (r *Rollout, found bool, err error)

	// GetRollout returns past or live rollout by id.
	GetRollout(id RolloutID) (r *Rollout, found bool, err error)
}
