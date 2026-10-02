package logtest_test

import (
	"testing"

	lognoop "github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/log/logtest"
)

// TestConformanceNoop proves the kit passes against the noop adapter.
func TestConformanceNoop(t *testing.T) {
	t.Parallel()

	logtest.Conformance(t, func(_ *testing.T) log.Logger {
		return lognoop.New()
	})
}
