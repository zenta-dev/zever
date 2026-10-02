package noop

import (
	"testing"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/observability/observabilitytest"
)

// TestNoopConformance proves the noop adapter honors the
// observability.Provider contract via the shared conformance kit.
// The adapter discards every span and measurement, so the kit
// asserts call shape, not export.
func TestNoopConformance(t *testing.T) {
	observabilitytest.Conformance(t, func(_ *testing.T) observability.Provider {
		return New()
	})
}
