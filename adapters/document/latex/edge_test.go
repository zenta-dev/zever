package latex

import (
	"testing"

	"github.com/zenta-dev/zever/core/document"
)

// TestEdgeRender_emptySource covers the zero-length source boundary: the
// compiler invocation must still run and produce output rather than error.
func TestEdgeRender_emptySource(t *testing.T) {
	stubBin(t, document.DefaultLatexCommand, fakeTexOK)

	d := mustOpen(t, document.Options{})

	if _, err := d.Render(t.Context(), nil, document.FormatPDF); err != nil {
		t.Fatalf("Render(nil) = %v, want nil", err)
	}
}
