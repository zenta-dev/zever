package observabilitytest_test

import (
	"testing"

	observabilitynoop "github.com/zenta-dev/zever/adapters/observability/noop"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/observability/observabilitytest"
)

// TestConformanceNoop proves the kit passes against the noop adapter.
func TestConformanceNoop(t *testing.T) {
	t.Parallel()

	observabilitytest.Conformance(t, func(_ *testing.T) observability.Provider {
		return observabilitynoop.New()
	})
}
