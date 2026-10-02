package noop

import (
	"testing"

	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/log/logtest"
)

// TestNoopConformance proves the noop adapter honors the log.Logger
// contract via the shared conformance kit. The adapter discards every
// event, so the kit asserts emission shape, not output.
func TestNoopConformance(t *testing.T) {
	logtest.Conformance(t, func(_ *testing.T) log.Logger {
		return New()
	})
}
