package latex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/document"
)

const (
	fakeTexOK      = "#!/bin/sh\nprintf '%%PDF-1.4 fake\\n' > doc.pdf\nexit 0\n"
	fakeTexFail    = "#!/bin/sh\necho 'font error: missing package' >&2\nexit 1\n"
	fakeTexSilent  = "#!/bin/sh\nexit 0\n"
	fakeTexHang    = "#!/bin/sh\nsleep 30\n"
	fakePPMOK      = "#!/bin/sh\nroot=\"\"\nfor a in \"$@\"; do root=\"$a\"; done\nprintf '\\211PNG\\r\\n\\032\\n fake' > \"$root.png\"\nprintf '\\377\\330\\377 fake' > \"$root.jpg\"\nexit 0\n"
	fakePPMFail    = "#!/bin/sh\necho 'converter boom' >&2\nexit 1\n"
	fakePPMNowrite = "#!/bin/sh\nexit 0\n"
	fakePPMBig     = "#!/bin/sh\nroot=\"\"\nfor a in \"$@\"; do root=\"$a\"; done\nhead -c 100000 /dev/zero > \"$root.png\"\nexit 0\n"
)

// fakeTexCount returns a stub compiler that records one line per pass to log
// (baked into the script: the child no longer inherits parent env vars).
func fakeTexCount(log string) string {
	return fmt.Sprintf("#!/bin/sh\nprintf '%%PDF-1.4 fake\\n' > doc.pdf\necho x >> %q\nexit 0\n", log)
}

// fakeTexArgs returns a stub compiler that records argv to log (baked into
// the script: the child no longer inherits parent env vars).
func fakeTexArgs(log string) string {
	return fmt.Sprintf("#!/bin/sh\nprintf '%%PDF-1.4 fake\\n' > doc.pdf\necho \"$@\" >> %q\nexit 0\n", log)
}

// fakeTexDump returns a stub compiler that records cwd, argv, and the full
// child environment to log so tests can assert the sandbox allowlist.
func fakeTexDump(log string) string {
	return fmt.Sprintf(`#!/bin/sh
printf '%%PDF-1.4 fake\n' > doc.pdf
{
  echo "cwd=$(pwd)"
  echo "args=$*"
  env | sort
} > %q
exit 0
`, log)
}

func stubBin(tb testing.TB, name, body string) {
	tb.Helper()
	dir := tb.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		tb.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		tb.Fatal(err)
	}
	tb.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func mustOpen(tb testing.TB, o document.Options) document.Document {
	tb.Helper()
	d, err := New(o)
	if err != nil {
		tb.Fatalf("New() err = %v", err)
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
	runsLog := filepath.Join(t.TempDir(), "runs.log")
	stubBin(t, "pdflatex-count", fakeTexCount(runsLog))

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
	argsLog := filepath.Join(t.TempDir(), "args.log")
	stubBin(t, "pdflatex-args", fakeTexArgs(argsLog))

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

func TestCompileSandboxEnv(t *testing.T) {
	dumpLog := filepath.Join(t.TempDir(), "sandbox.log")
	stubBin(t, "pdflatex-dump", fakeTexDump(dumpLog))

	t.Setenv("HOME", filepath.Join(t.TempDir(), "parent-home"))
	t.Setenv("ZEVER_LATEX_TEST_SECRET", "topsecret-value-xyz")
	t.Setenv("ZEVER_LATEX_TEST_LEAK", "leaked-value-xyz")

	d := mustOpen(t, document.Options{LatexCommand: "pdflatex-dump"})
	if _, err := d.Render(t.Context(), []byte("src"), document.FormatPDF); err != nil {
		t.Fatalf("Render() err = %v", err)
	}

	b, err := os.ReadFile(dumpLog)
	if err != nil {
		t.Fatal(err)
	}
	dump := string(b)

	for _, tc := range []struct{ name, want string }{
		{"-no-shell-escape flag", "-no-shell-escape"},
		{"paranoid openin_any", "openin_any=p"},
		{"paranoid openout_any", "openout_any=p"},
		{"PATH allowlist", "PATH="},
	} {
		if !strings.Contains(dump, tc.want) {
			t.Errorf("child sandbox missing %s; dump:\n%s", tc.name, dump)
		}
	}

	for _, leak := range []string{"ZEVER_LATEX_TEST_SECRET", "topsecret-value-xyz", "ZEVER_LATEX_TEST_LEAK", "leaked-value-xyz", "parent-home"} {
		if strings.Contains(dump, leak) {
			t.Errorf("child sandbox leaks %q; dump:\n%s", leak, dump)
		}
	}

	cwd := ""
	home := ""
	for _, line := range strings.Split(dump, "\n") {
		if strings.HasPrefix(line, "cwd=") {
			cwd = strings.TrimPrefix(line, "cwd=")
		}
		if strings.HasPrefix(line, "HOME=") {
			home = strings.TrimPrefix(line, "HOME=")
		}
	}
	if cwd == "" {
		t.Fatalf("dump lacks cwd; dump:\n%s", dump)
	}
	if !strings.HasPrefix(cwd, filepath.Join(os.TempDir(), "latex-")) {
		t.Errorf("cwd = %q, want render temp dir %q", cwd, filepath.Join(os.TempDir(), "latex-"))
	}
	if home != cwd {
		t.Errorf("HOME = %q, want render dir %q", home, cwd)
	}
}

func TestRenderTimeout(t *testing.T) {
	stubBin(t, "pdflatex-hang", fakeTexHang)

	d := mustOpen(t, document.Options{LatexCommand: "pdflatex-hang", Timeout: 200 * time.Millisecond})
	_, err := d.Render(t.Context(), []byte("src"), document.FormatPDF)
	if !errors.Is(err, ErrRenderFailed) {
		t.Fatalf("Render() err = %v, want ErrRenderFailed", err)
	}
	if !strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("Render() err = %v, want signal: killed", err)
	}
}

// texLiveVersion extracts the TeX Live year from `pdflatex --version`, or
// skips the test when the binary is absent or unrecognizable.
func texLiveVersion(tb testing.TB) int {
	tb.Helper()

	//nolint:gosec // fixed argv; read-only --version probe for the feature-skip check.
	out, err := exec.CommandContext(tb.Context(), document.DefaultLatexCommand, "--version").Output()
	if err != nil {
		tb.Skipf("%s --version: %v", document.DefaultLatexCommand, err)
	}
	m := regexp.MustCompile(`TeX Live (\d{4})`).FindSubmatch(out)
	if m == nil {
		tb.Skipf("unrecognized %s --version output: %q", document.DefaultLatexCommand, out)
	}
	year, err := strconv.Atoi(string(m[1]))
	if err != nil {
		tb.Fatalf("parse TeX Live year from %q: %v", out, err)
	}
	return year
}

func TestE2EParanoidInput(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath(document.DefaultLatexCommand); err != nil {
		t.Skip("pdflatex not installed")
	}
	// TeX Live >= 2026 declares openin_any a no-op (kpse_in_name_ok always
	// returns true), so absolute \input cannot be restricted there; the
	// openin_any=p env still protects older TeX Live releases.
	if year := texLiveVersion(t); year >= 2026 {
		t.Skipf("pdflatex is TeX Live %d, where openin_any is a no-op upstream", year)
	}

	source := []byte(`\documentclass{article}\begin{document}\input{/etc/hostname}\end{document}`)
	d := mustOpen(t, document.Options{Timeout: 2 * time.Minute})
	_, err := d.Render(t.Context(), source, document.FormatPDF)
	if !errors.Is(err, ErrRenderFailed) {
		t.Fatalf("Render() err = %v, want ErrRenderFailed for blocked absolute input", err)
	}
}

func TestE2EWriteRestricted(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath(document.DefaultLatexCommand); err != nil {
		t.Skip("pdflatex not installed")
	}

	leak := filepath.Join(t.TempDir(), "leak.tex")
	source := []byte("\\documentclass{article}\\begin{document}\\newwrite\\w\\immediate\\openout\\w=" + leak + "\\immediate\\write\\w{hi}\\immediate\\closeout\\w\nx\n\\end{document}")
	d := mustOpen(t, document.Options{Timeout: 2 * time.Minute})
	_, err := d.Render(t.Context(), source, document.FormatPDF)
	if !errors.Is(err, ErrRenderFailed) {
		t.Fatalf("Render() err = %v, want ErrRenderFailed for blocked openout", err)
	}
	if _, statErr := os.Stat(leak); !os.IsNotExist(statErr) {
		t.Errorf("openout wrote %s despite openout_any=p", leak)
	}
}

func TestOversizedPNGOutput(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)
	stubBin(t, "pdftoppm-big", fakePPMBig)

	d := mustOpen(t, document.Options{LatexPDFToPPM: "pdftoppm-big", MaxOutputBytes: 100})
	_, err := d.Render(t.Context(), []byte("src"), document.FormatPNG)
	var sle document.SizeLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("Render() err = %v, want SizeLimitError", err)
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
	var sle document.SizeLimitError
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
	var ufe document.UnsupportedFormatError
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
	var sle document.SizeLimitError
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
