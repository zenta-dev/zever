package local

import (
	"testing"

	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/core/document/documenttest"
)

// TestOpenDirect proves direct construction opens a working backend.
func TestOpenDirect(t *testing.T) {
	t.Parallel()

	d, err := New(document.Options{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	defer func() { _ = d.Close() }()

	if _, err := d.Render(t.Context(), []byte("<h1>wiring</h1>"), document.OutputFormat("docx")); err == nil {
		t.Error("Render(docx) = nil, want unsupported-format error")
	}
}

// TestConformance runs the shared document kit against local.
func TestConformance(t *testing.T) {
	t.Parallel()

	documenttest.Conformance(t, func(t *testing.T) document.Document {
		t.Helper()

		// Cold Chrome launch on CI runners can exceed the production
		// default; use the same tripled budget local_test.go uses. The
		// kit asserts output shape, not speed.
		d, err := New(document.Options{Timeout: renderTestTimeout})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = d.Close() })

		return d
	})
}
