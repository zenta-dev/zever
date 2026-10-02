package stdout

import (
	"io"
	"testing"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/observability/observabilitytest"
)

// TestStdoutConformance proves the stdout adapter honors the
// observability.Provider contract via the shared conformance kit.
// Output goes to io.Discard so conformance stays silent.
func TestStdoutConformance(t *testing.T) {
	observabilitytest.Conformance(t, func(t *testing.T) observability.Provider {
		t.Helper()

		p, err := NewWithWriter(observability.Options{ServiceName: "kit"}, io.Discard)
		if err != nil {
			t.Fatalf("NewWithWriter() error = %v", err)
		}

		t.Cleanup(func() { _ = p.Shutdown(t.Context()) })

		return p
	})
}
