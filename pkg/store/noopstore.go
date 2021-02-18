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
