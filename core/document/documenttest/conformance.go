// Package documenttest provides the conformance kit third-party document adapters run to prove backend parity.
package documenttest

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/document"
)

// latexSample is the shared happy-path source. It is a minimal LaTeX document
// so the latex backend compiles it, and the local backend renders any bytes
// as text through headless Chrome, so the same source exercises both.
const latexSample = `\documentclass{article}\begin{document}hi\end{document}`

// Conformance verifies factory-built document backends implement the
// document.Document contract: PDF render happy-path with %PDF- magic,
// unsupported-format and source-too-large sentinels, canceled-context
// propagation, and Close. Each subtest takes a fresh instance from factory so
// cases stay isolated. Tests are deterministic and touch no network.
func Conformance(t *testing.T, factory func(t *testing.T) document.Document) {
	t.Helper()

	t.Run("RenderPDF", func(t *testing.T) { conformanceRenderPDF(t, factory) })
	t.Run("UnsupportedFormat", func(t *testing.T) { conformanceUnsupportedFormat(t, factory) })
	t.Run("SourceTooLarge", func(t *testing.T) { conformanceSourceTooLarge(t, factory) })
	t.Run("CanceledContext", func(t *testing.T) { conformanceCanceledContext(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceRenderPDF(t *testing.T, factory func(t *testing.T) document.Document) {
	t.Helper()

	out, err := factory(t).Render(t.Context(), []byte(latexSample), document.FormatPDF)
	if err != nil {
		t.Fatalf("Render(PDF) error = %v", err)
	}

	if len(out) < 5 || string(out[:5]) != "%PDF-" {
		t.Errorf("Render(PDF) missing %%PDF- magic, got %q", out[:min(8, len(out))])
	}
}

func conformanceUnsupportedFormat(t *testing.T, factory func(t *testing.T) document.Document) {
	t.Helper()

	_, err := factory(t).Render(t.Context(), []byte(latexSample), document.OutputFormat("docx"))

	var ufErr document.UnsupportedFormatError
	if !errors.As(err, &ufErr) {
		t.Fatalf("Render(docx) err = %T %v, want *UnsupportedFormatError", err, err)
	}

	if ufErr.Format != document.OutputFormat("docx") {
		t.Errorf("UnsupportedFormatError.Format = %q, want docx", ufErr.Format)
	}

	if !errors.Is(err, document.ErrUnsupportedFormat) {
		t.Errorf("errors.Is(err, ErrUnsupportedFormat) = false (err = %v)", err)
	}
}

func conformanceSourceTooLarge(t *testing.T, factory func(t *testing.T) document.Document) {
	t.Helper()

	big := make([]byte, document.DefaultMaxSourceBytes+1)

	_, err := factory(t).Render(t.Context(), big, document.FormatPDF)

	var slErr document.SizeLimitError
	if !errors.As(err, &slErr) {
		t.Fatalf("Render(oversized) err = %T %v, want *SizeLimitError", err, err)
	}

	if slErr.Size != len(big) {
		t.Errorf("SizeLimitError.Size = %d, want %d", slErr.Size, len(big))
	}

	if slErr.Limit != document.DefaultMaxSourceBytes {
		t.Errorf("SizeLimitError.Limit = %d, want %d", slErr.Limit, document.DefaultMaxSourceBytes)
	}

	if !errors.Is(err, document.ErrSourceTooLarge) {
		t.Errorf("errors.Is(err, ErrSourceTooLarge) = false (err = %v)", err)
	}
}

func conformanceCanceledContext(t *testing.T, factory func(t *testing.T) document.Document) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := factory(t).Render(ctx, []byte(latexSample), document.FormatPDF)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Render(canceled) err = %v, want errors.Is context.Canceled", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) document.Document) {
	t.Helper()

	d := factory(t)

	if err := d.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := d.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
