package latex

import (
	"testing"

	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/core/document/documenttest"
)

// TestOpenRegister proves Register wiring plus Open round-trip. It uses a
// test-only adapter name so it cannot collide with other registrations in
// this binary; the kit factory below opens via New directly. Open requires
// the system pdflatex binary, resolved via LookPath.
func TestOpenRegister(t *testing.T) {
	t.Parallel()

	const adapter = document.Adapter("kit-document-latex")

	if err := document.Register(adapter, New); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	d, err := document.Open(adapter, document.Options{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	defer func() { _ = d.Close() }()

	if _, err := d.Render(t.Context(), []byte(`x`), document.OutputFormat("docx")); err == nil {
		t.Error("Render(docx) = nil, want unsupported-format error")
	}
}

// TestConformance runs the shared document kit against latex.
func TestConformance(t *testing.T) {
	t.Parallel()

	documenttest.Conformance(t, func(t *testing.T) document.Document {
		t.Helper()

		d, err := New(document.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = d.Close() })

		return d
	})
}
