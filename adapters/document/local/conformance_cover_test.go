package local

import (
	"testing"

	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/core/document/documenttest"
)

// TestOpenRegister proves Register wiring plus Open round-trip. It uses a
// test-only adapter name: registering document.Local itself would collide
// with example tests' strict Register in this same binary, so the kit
// factory below opens via New directly.
func TestOpenRegister(t *testing.T) {
	t.Parallel()

	const adapter = document.Adapter("kit-document-local")

	if err := document.Register(adapter, New); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	d, err := document.Open(adapter, document.Options{})
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
		// default; use the same doubled budget local_test.go uses. The
		// kit asserts output shape, not speed.
		d, err := New(document.Options{Timeout: renderTestTimeout})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = d.Close() })

		return d
	})
}
