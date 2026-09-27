package latex

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/document"
)

const (
	fakeTexOK      = "#!/bin/sh\nprintf '%%PDF-1.4 fake\\n' > doc.pdf\nexit 0\n"
	fakeTexCount   = "#!/bin/sh\nprintf '%%PDF-1.4 fake\\n' > doc.pdf\necho x >> \"$RUNSLOG\"\nexit 0\n"
	fakeTexFail    = "#!/bin/sh\necho 'font error: missing package' >&2\nexit 1\n"
	fakeTexSilent  = "#!/bin/sh\nexit 0\n"
	fakePPMOK      = "#!/bin/sh\nroot=\"\"\nfor a in \"$@\"; do root=\"$a\"; done\nprintf '\\211PNG\\r\\n\\032\\n fake' > \"$root.png\"\nprintf '\\377\\330\\377 fake' > \"$root.jpg\"\nexit 0\n"
	fakePPMFail    = "#!/bin/sh\necho 'converter boom' >&2\nexit 1\n"
	fakePPMNowrite = "#!/bin/sh\nexit 0\n"
	fakeTexArgs    = "#!/bin/sh\nprintf '%%PDF-1.4 fake\\n' > doc.pdf\necho \"$@\" >> \"$ARGSLOG\"\nexit 0\n"
)

func stubBin(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func mustOpen(t *testing.T, o document.Options) document.Document {
	t.Helper()
	d, err := New(o)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	return d
}

func TestOpenMissingCommand(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := New(document.Options{})
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("New() err = %v, want exec.ErrNotFound", err)
	}
}

func TestOpenInvalidOptions(t *testing.T) {
	t.Parallel()
	_, err := New(document.Options{Quality: 101})
	if !errors.Is(err, document.ErrInvalidOptions) {
		t.Fatalf("New() err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpenDefaults(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)

	d := mustOpen(t, document.Options{})
	drv, ok := d.(*driver)
	if !ok {
		t.Fatalf("New() type = %T, want *driver", d)
	}
	if drv.command == "" || !strings.HasSuffix(drv.command, "pdflatex") {
		t.Errorf("command = %q, want resolved pdflatex", drv.command)
	}
	if drv.converter != document.DefaultLatexConverter {
		t.Errorf("converter = %q, want %q", drv.converter, document.DefaultLatexConverter)
	}
	if drv.runs != 1 {
		t.Errorf("runs = %d, want 1", drv.runs)
	}
	if drv.dpi != document.DefaultLatexDPI {
		t.Errorf("dpi = %d, want %d", drv.dpi, document.DefaultLatexDPI)
	}
	if drv.quality != document.DefaultLatexQuality {
		t.Errorf("quality = %d, want %d", drv.quality, document.DefaultLatexQuality)
	}
	if drv.timeout != document.DefaultTimeout {
		t.Errorf("timeout = %v, want %v", drv.timeout, document.DefaultTimeout)
	}
	if drv.maxOutput != document.DefaultMaxOutputBytes {
		t.Errorf("maxOutput = %d, want %d", drv.maxOutput, document.DefaultMaxOutputBytes)
	}
	if drv.tmpDir != "" {
		t.Errorf("tmpDir = %q, want empty", drv.tmpDir)
	}
}

func TestOpenCustom(t *testing.T) {
	stubBin(t, "my-tex", fakeTexOK)

	tmp := t.TempDir()
	d := mustOpen(t, document.Options{
		LatexCommand:   "my-tex",
		LatexPDFToPPM:  "custom-conv-xyz",
		LatexRuns:      3,
		DPI:            200,
		Quality:        50,
		Timeout:        time.Second,
		TmpDir:         tmp,
		MaxOutputBytes: 1234,
	})
	drv, ok := d.(*driver)
	if !ok {
		t.Fatalf("New() type = %T, want *driver", d)
	}
	if !strings.HasSuffix(drv.command, "my-tex") {
		t.Errorf("command = %q, want suffix my-tex", drv.command)
	}
	if drv.converter != "custom-conv-xyz" {
		t.Errorf("converter = %q, want custom-conv-xyz", drv.converter)
	}
	if drv.runs != 3 {
		t.Errorf("runs = %d, want 3", drv.runs)
	}
	if drv.dpi != 200 {
		t.Errorf("dpi = %d, want 200", drv.dpi)
	}
	if drv.quality != 50 {
		t.Errorf("quality = %d, want 50", drv.quality)
	}
	if drv.timeout != time.Second {
		t.Errorf("timeout = %v, want 1s", drv.timeout)
	}
	if drv.tmpDir != tmp {
		t.Errorf("tmpDir = %q, want %q", drv.tmpDir, tmp)
	}
	if drv.maxOutput != 1234 {
		t.Errorf("maxOutput = %d, want 1234", drv.maxOutput)
	}
}

func TestRenderPDF(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)

	d := mustOpen(t, document.Options{})
	out, err := d.Render(t.Context(), []byte(`\documentclass{article}\begin{document}hi\end{document}`), document.FormatPDF)
	if err != nil {
		t.Fatalf("Render() err = %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Errorf("Render() = %q, want %%PDF- prefix", out)
	}
}

func TestRunsHonored(t *testing.T) {
	stubBin(t, "pdflatex-count", fakeTexCount)
	runsLog := filepath.Join(t.TempDir(), "runs.log")
	t.Setenv("RUNSLOG", runsLog)

	count := func() int {
		b, err := os.ReadFile(runsLog)
		if err != nil {
			if os.IsNotExist(err) {
				return 0
			}
			t.Fatal(err)
		}
		return strings.Count(string(b), "\n")
	}
	for _, tc := range []struct {
		runs int
		want int
	}{{runs: 3, want: 3}, {runs: 0, want: 1}, {runs: 99, want: 5}} {
		before := count()
		d := mustOpen(t, document.Options{LatexCommand: "pdflatex-count", LatexRuns: tc.runs})
		if _, err := d.Render(t.Context(), []byte("src"), document.FormatPDF); err != nil {
			t.Fatalf("Render(runs=%d) err = %v", tc.runs, err)
		}
		if got := count() - before; got != tc.want {
			t.Errorf("Render(runs=%d) passes = %d, want %d", tc.runs, got, tc.want)
		}
	}
}

func TestCompileArgsIncludeNoShellEscape(t *testing.T) {
	stubBin(t, "pdflatex-args", fakeTexArgs)
	argsLog := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("ARGSLOG", argsLog)

	d := mustOpen(t, document.Options{LatexCommand: "pdflatex-args"})
	if _, err := d.Render(t.Context(), []byte("src"), document.FormatPDF); err != nil {
		t.Fatalf("Render() err = %v", err)
	}

	b, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "-no-shell-escape") {
		t.Fatalf("compile argv = %q, want it to contain -no-shell-escape", string(b))
	}
}

func TestRenderFailure(t *testing.T) {
	stubBin(t, "pdflatex-fail", fakeTexFail)

	d := mustOpen(t, document.Options{LatexCommand: "pdflatex-fail"})
	out, err := d.Render(t.Context(), []byte("src"), document.FormatPDF)
	if !errors.Is(err, ErrRenderFailed) {
		t.Fatalf("Render() err = %v, want ErrRenderFailed", err)
	}
	if !strings.Contains(err.Error(), "font error") {
		t.Errorf("Render() err = %q, want stderr in message", err)
	}
	if out != nil {
		t.Errorf("Render() output = %q, want nil on error", out)
	}
}

func TestMissingOutput(t *testing.T) {
	stubBin(t, "pdflatex-silent", fakeTexSilent)

	d := mustOpen(t, document.Options{LatexCommand: "pdflatex-silent"})
	_, err := d.Render(t.Context(), []byte("src"), document.FormatPDF)
	if err == nil || !strings.Contains(err.Error(), "latex: read output") {
		t.Fatalf("Render() err = %v, want latex: read output", err)
	}
}

func TestOversizedOutput(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)

	d := mustOpen(t, document.Options{MaxOutputBytes: 1})
	_, err := d.Render(t.Context(), []byte("src"), document.FormatPDF)
	var sle *document.SizeLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("Render() err = %v, want SizeLimitError", err)
	}
}

func TestRenderPNG(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)
	stubBin(t, "pdftoppm", fakePPMOK)

	d := mustOpen(t, document.Options{})
	out, err := d.Render(t.Context(), []byte("src"), document.FormatPNG)
	if err != nil {
		t.Fatalf("Render() err = %v", err)
	}
	if !bytes.HasPrefix(out, []byte("\x89PNG")) {
		t.Errorf("Render() = %q, want PNG magic", out)
	}
}

func TestRenderJPG(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)
	stubBin(t, "pdftoppm", fakePPMOK)

	d := mustOpen(t, document.Options{})
	out, err := d.Render(t.Context(), []byte("src"), document.FormatJPG)
	if err != nil {
		t.Fatalf("Render() err = %v", err)
	}
	if !bytes.HasPrefix(out, []byte("\xff\xd8")) {
		t.Errorf("Render() = %q, want JPEG magic", out)
	}
}

func TestMissingConverter(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)

	d := mustOpen(t, document.Options{LatexPDFToPPM: "no-such-bin-xyz"})
	_, err := d.Render(t.Context(), []byte("src"), document.FormatPNG)
	if !errors.Is(err, ErrMissingConverter) {
		t.Fatalf("Render() err = %v, want ErrMissingConverter", err)
	}
}

func TestConverterFailure(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)
	stubBin(t, "pdftoppm-fail", fakePPMFail)

	d := mustOpen(t, document.Options{LatexPDFToPPM: "pdftoppm-fail"})
	_, err := d.Render(t.Context(), []byte("src"), document.FormatPNG)
	if !errors.Is(err, ErrRenderFailed) {
		t.Fatalf("Render() err = %v, want ErrRenderFailed", err)
	}
}

func TestConvertReadImageFailure(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)
	stubBin(t, "pdftoppm-nowrite", fakePPMNowrite)

	d := mustOpen(t, document.Options{LatexPDFToPPM: "pdftoppm-nowrite"})
	_, err := d.Render(t.Context(), []byte("src"), document.FormatPNG)
	if err == nil || !strings.Contains(err.Error(), "latex: read image") {
		t.Fatalf("Render() err = %v, want latex: read image", err)
	}
}

func TestConvertWritePDFFailure(t *testing.T) {
	stubBin(t, "pdftoppm", fakePPMOK)

	d := &driver{command: "pdflatex", converter: "pdftoppm", runs: 1, dpi: 150, quality: 80, timeout: time.Minute, maxOutput: 1 << 20}
	_, err := d.convert(t.Context(), filepath.Join(t.TempDir(), "no-such-dir"), []byte("%PDF-1.4 fake"), document.FormatPNG)
	if err == nil || !strings.Contains(err.Error(), "latex: write pdf") {
		t.Fatalf("convert() err = %v, want latex: write pdf", err)
	}
}

func TestBadFormat(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)

	d := mustOpen(t, document.Options{})
	_, err := d.Render(t.Context(), []byte("src"), document.OutputFormat("gif"))
	var ufe *document.UnsupportedFormatError
	if !errors.As(err, &ufe) {
		t.Fatalf("Render() err = %v, want UnsupportedFormatError", err)
	}
	if !errors.Is(err, document.ErrUnsupportedFormat) {
		t.Errorf("Render() err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestOversizedSource(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)

	d := mustOpen(t, document.Options{})
	src := make([]byte, document.DefaultMaxSourceBytes+1)
	_, err := d.Render(t.Context(), src, document.FormatPDF)
	var sle *document.SizeLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("Render() err = %v, want SizeLimitError", err)
	}
	if sle.Size != len(src) || sle.Limit != document.DefaultMaxSourceBytes {
		t.Errorf("SizeLimitError = {%d %d}, want {%d %d}", sle.Size, sle.Limit, len(src), document.DefaultMaxSourceBytes)
	}
}

func TestCanceledContext(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)

	d := mustOpen(t, document.Options{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := d.Render(ctx, []byte("src"), document.FormatPDF)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Render() err = %v, want context.Canceled", err)
	}
}

func TestTempDirFailure(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)

	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := mustOpen(t, document.Options{TmpDir: blocker})
	_, err := d.Render(t.Context(), []byte("src"), document.FormatPDF)
	if err == nil || !strings.Contains(err.Error(), "latex: temp dir") {
		t.Fatalf("Render() err = %v, want latex: temp dir", err)
	}
}

func TestClose(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)

	d := mustOpen(t, document.Options{})
	if err := d.Close(); err != nil {
		t.Fatalf("Close() err = %v, want nil", err)
	}
}

func TestConcurrent(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)
	stubBin(t, "pdftoppm", fakePPMOK)

	d := mustOpen(t, document.Options{})
	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := d.Render(t.Context(), []byte("src"), document.FormatPDF)
			if err != nil {
				errs[i] = err
				return
			}
			if !bytes.HasPrefix(out, []byte("%PDF-")) {
				errs[i] = errors.New("missing %%PDF- prefix")
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("Render(%d) err = %v", i, err)
		}
	}
}

func TestE2ERealPDF(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat("/usr/bin/pdflatex"); err != nil {
		t.Skip("pdflatex not installed")
	}
	d := mustOpen(t, document.Options{LatexCommand: "/usr/bin/pdflatex", Timeout: 2 * time.Minute})
	out, err := d.Render(t.Context(), []byte(`\documentclass{article}\begin{document}hello\end{document}`), document.FormatPDF)
	if err != nil {
		t.Fatalf("Render() err = %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Error("Render() lacks %PDF- prefix")
	}
}

func TestE2ERealPNG(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat("/usr/bin/pdflatex"); err != nil {
		t.Skip("pdflatex not installed")
	}
	if _, err := os.Stat("/usr/bin/pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}
	d := mustOpen(t, document.Options{LatexCommand: "/usr/bin/pdflatex", LatexPDFToPPM: "/usr/bin/pdftoppm", Timeout: 2 * time.Minute})
	out, err := d.Render(t.Context(), []byte(`\documentclass{article}\begin{document}hello\end{document}`), document.FormatPNG)
	if err != nil {
		t.Fatalf("Render() err = %v", err)
	}
	if !bytes.HasPrefix(out, []byte("\x89PNG")) {
		t.Errorf("Render() lacks PNG magic")
	}
}
