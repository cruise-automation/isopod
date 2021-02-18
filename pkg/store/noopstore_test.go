package store

import "testing"

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
