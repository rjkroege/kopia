package clock

import "testing"

// TestGateMustFail deliberately fails to verify that branch protection blocks merging.
func TestGateMustFail(t *testing.T) {
	t.Fatal("deliberate failure: branch protection gate check")
}
