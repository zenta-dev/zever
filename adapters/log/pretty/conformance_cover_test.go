package pretty

import (
	"io"
	"testing"

	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/log/logtest"
)

// TestPrettyConformance proves the pretty adapter honors the log.Logger
// contract via the shared conformance kit. Output goes to io.Discard
// so conformance stays silent.
func TestPrettyConformance(t *testing.T) {
	logtest.Conformance(t, func(_ *testing.T) log.Logger {
		return NewWithWriter(log.Options{}, io.Discard)
	})
}
