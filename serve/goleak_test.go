package serve

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain verifies no goroutine outlives the package tests: every
// listener, scheduler tick, and signal context must shut down cleanly.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
