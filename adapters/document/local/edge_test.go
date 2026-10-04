package local

import (
	"testing"

	"github.com/zenta-dev/zever/core/document"
)

// TestEdgeReopenAfterClose covers the Open -> Close -> re-Open lifecycle:
// closing one driver must not poison a freshly constructed one.
func TestEdgeReopenAfterClose(t *testing.T) {
	t.Parallel()

	first, err := New(document.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if closeErr := first.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	second, err := New(document.Options{})
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}

	t.Cleanup(func() { _ = second.Close() })

	if _, err := second.Render(t.Context(), []byte("<html></html>"), document.OutputFormat("bogus")); err == nil {
		t.Fatal("Render(bogus) = nil, want UnsupportedFormatError")
	}
}
