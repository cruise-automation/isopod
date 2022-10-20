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

// NoopStore implements Store interface for no-op store.
// It does not store rollout and addon run information anywhere.
type NoopStore struct{}

// CreateRollout only returns a new empty Rollout.
func (NoopStore) CreateRollout() (*Rollout, error) {
	return &Rollout{}, nil
}

// PutAddonRun is a noop. It returns an empty string RunID.
func (NoopStore) PutAddonRun(id RolloutID, _ *AddonRun) (RunID, error) {
	return "", nil
}

// CompleteRollout is a noop.
func (NoopStore) CompleteRollout(id RolloutID) error { return nil }

// GetLive returns a nil Rollout and `false` for `found`.
func (NoopStore) GetLive() (r *Rollout, found bool, err error) {
	return nil, false, nil
}

// GetRollout returns a nil Rollout and `false` for `found`.
func (NoopStore) GetRollout(id RolloutID) (r *Rollout, found bool, err error) {
	return nil, false, nil
}
