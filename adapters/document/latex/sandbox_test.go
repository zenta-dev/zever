package latex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/document"
)

// fakeBwrap logs its argv and cwd, then execs the wrapped command after "--".
const fakeBwrap = `#!/bin/sh
{ echo "cwd=$(pwd)"; for a in "$@"; do echo "arg=$a"; done; } > LOGPATH
while [ "$1" != "--" ]; do shift; done
shift
exec "$@"
`

func fakeDeps(bwrap string, lookErr, probeErr error, warns *[]string) deps {
	return deps{
		lookPath: func(string) (string, error) { return bwrap, lookErr },
		probe:    func(context.Context, string) error { return probeErr },
		warn:     func(m string) { *warns = append(*warns, m) },
	}
}

func TestSandboxWrapsCommand(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)
	log := filepath.Join(t.TempDir(), "bwrap.log")
	bwrap := filepath.Join(t.TempDir(), "bwrap")
	if err := os.WriteFile(bwrap, []byte(strings.Replace(fakeBwrap, "LOGPATH", `"`+log+`"`, 1)), 0o700); err != nil { // test stub must be executable.
		t.Fatal(err)
	}

	var warns []string
	d, err := newDriver(document.Options{LatexSandbox: document.LatexSandboxRequired}, fakeDeps(bwrap, nil, nil, &warns))
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := d.Render(t.Context(), []byte("src"), document.FormatPDF); rerr != nil {
		t.Fatalf("Render() err = %v", rerr)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	var cwd string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if v, ok := strings.CutPrefix(l, "arg="); ok {
			args = append(args, v)
		} else if v, ok := strings.CutPrefix(l, "cwd="); ok {
			cwd = v
		}
	}
	for _, want := range []string{"--unshare-all", "--die-with-parent", "--bind", "--chdir", "-no-shell-escape"} {
		if !slices.Contains(args, want) {
			t.Errorf("bwrap argv missing %q: %v", want, args)
		}
	}
	// only the render dir is writable
	if n := strings.Count(strings.Join(args, " "), "--bind "); n != 1 {
		t.Errorf("writable --bind count = %d, want 1: %v", n, args)
	}
	i := slices.Index(args, "--chdir")
	if i < 0 || args[i+1] != cwd {
		t.Errorf("--chdir/cwd mismatch: args=%v cwd=%q", args, cwd)
	}
	if slices.Contains(args, "/etc") {
		t.Errorf("/etc must not be bound wholesale: %v", args)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestSandboxRequiredUnavailable(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)
	for name, dp := range map[string]func(*[]string) deps{
		"missing": func(w *[]string) deps { return fakeDeps("", exec.ErrNotFound, nil, w) },
		"probe":   func(w *[]string) deps { return fakeDeps("/x/bwrap", nil, errors.New("userns disabled"), w) },
	} {
		var warns []string
		_, err := newDriver(document.Options{LatexSandbox: document.LatexSandboxRequired}, dp(&warns))
		if !errors.Is(err, ErrSandboxUnavailable) {
			t.Errorf("%s: err = %v, want ErrSandboxUnavailable", name, err)
		}
	}
}

func TestSandboxAutoFallbackWarnsOnce(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)
	var warns []string
	d, err := newDriver(document.Options{}, fakeDeps("", exec.ErrNotFound, nil, &warns))
	if err != nil {
		t.Fatal(err)
	}
	if d.sb != nil {
		t.Fatal("sandbox should be nil on fallback")
	}
	for range 3 {
		if _, err := d.Render(t.Context(), []byte("src"), document.FormatPDF); err != nil {
			t.Fatal(err)
		}
	}
	if len(warns) != 1 {
		t.Errorf("warnings = %d, want 1: %v", len(warns), warns)
	}
}

func TestSandboxOffSkipsLookup(t *testing.T) {
	stubBin(t, "pdflatex", fakeTexOK)
	dp := deps{lookPath: func(string) (string, error) { t.Error("lookPath called"); return "", nil }}
	if _, err := newDriver(document.Options{LatexSandbox: document.LatexSandboxOff}, dp); err != nil {
		t.Fatal(err)
	}
}

func TestSandboxWrapArgs(t *testing.T) {
	sb := &sandbox{bwrap: "/usr/bin/bwrap", binds: []string{"/usr"}}
	name, argv := sb.wrap("/work/x", "/usr/bin/pdflatex", []string{"-a"})
	if name != "/usr/bin/bwrap" {
		t.Fatalf("name = %q", name)
	}
	tail := argv[len(argv)-3:]
	if !slices.Equal(tail, []string{"--", "/usr/bin/pdflatex", "-a"}) {
		t.Errorf("tail = %v", tail)
	}
	if !strings.Contains(strings.Join(argv, " "), "--bind /work/x /work/x") {
		t.Errorf("render dir not bound writable: %v", argv)
	}
}

func TestScanSource(t *testing.T) {
	cases := []struct {
		name string
		src  string
		ok   bool
	}{
		{"plain", `\documentclass{article}\begin{document}hi\end{document}`, true},
		{"relative input", `\input{chapters/one}`, true},
		{"relative include", `\include{one}`, true},
		{"bare relative input", "\\input chapter.tex\n", true},
		{"macro arg", `\input\jobname.aux`, true},
		{"absolute input", `\input{/etc/hostname}`, false},
		{"bare absolute input", "\\input /etc/passwd\n", false},
		{"dotdot input", `\input{../secret}`, false},
		{"tilde input", `\input{~/secret}`, false},
		{"pipe input", `\input{|cat /etc/passwd}`, false},
		{"windows abs", `\input{C:\Windows\win.ini}`, false},
		{"include abs", `\include{/etc/passwd}`, false},
		{"opt arg", `\input[x]{/etc/passwd}`, false},
		{"star", `\input*{/etc/passwd}`, false},
		{"openin abs", `\openin5=/etc/passwd`, false},
		{"openin macro stream", `\openin\f={../x}`, false},
		{"openin rel", `\openin5=data.txt`, true},
		{"openout abs", `\immediate\openout3=/tmp/x`, false},
		{"openout rel", `\immediate\openout3=out.txt`, true},
		{"write18", `\immediate\write18{ls}`, false},
		{"write18 spaced", `\write 18{ls}`, false},
		{"write stream", `\immediate\write3{hi}`, true},
		{"shellesc", `\usepackage{shellesc}`, false},
		{"shellesc list", `\usepackage[a=b]{graphicx, shellesc}`, false},
		{"minted", `\RequirePackage{minted}`, false},
		{"safe package", `\usepackage{graphicx,amsmath}`, true},
		{"commented out", "% \\input{/etc/passwd}\nhello", true},
		{"escaped percent", `\%\input{/etc/passwd}`, false},
		{"inputminted not input", `\inputminted{c}{x.c}`, true},
		{"verbatiminput abs", `\verbatiminput{/etc/passwd}`, false},
		{"InputIfFileExists", `\InputIfFileExists{../x}{}{}`, false},
	}
	for _, tc := range cases {
		err := scanSource([]byte(tc.src))
		if tc.ok && err != nil {
			t.Errorf("%s: err = %v, want nil", tc.name, err)
		}
		if !tc.ok && !errors.Is(err, ErrUnsafeSource) {
			t.Errorf("%s: err = %v, want ErrUnsafeSource", tc.name, err)
		}
	}
}

func TestRenderRejectsUnsafeSource(t *testing.T) {
	log := filepath.Join(t.TempDir(), "ran.log")
	stubBin(t, "pdflatex-ran", fakeTexCount(log))
	src := []byte(`\input{/etc/hostname}`)

	d := mustOpen(t, document.Options{LatexCommand: "pdflatex-ran"})
	if _, err := d.Render(t.Context(), src, document.FormatPDF); !errors.Is(err, ErrUnsafeSource) {
		t.Fatalf("err = %v, want ErrUnsafeSource", err)
	}
	if _, err := os.Stat(log); err == nil {
		t.Error("compiler ran despite unsafe source")
	}

	d = mustOpen(t, document.Options{LatexCommand: "pdflatex-ran", LatexAllowUnsafeSource: true})
	if _, err := d.Render(t.Context(), src, document.FormatPDF); err != nil {
		t.Fatalf("allow_unsafe_source Render() err = %v", err)
	}
}

func TestOutputCapChecksSizeBeforeReading(t *testing.T) {
	big := filepath.Join(t.TempDir(), "big.pdf")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if terr := f.Truncate(1 << 30); terr != nil { // sparse: no real memory/disk
		t.Fatal(terr)
	}
	_ = f.Close()

	d := &driver{maxOutput: 1 << 20}
	_, err = d.readCapped(big, "output")
	var sle document.SizeLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("err = %v, want SizeLimitError", err)
	}
	if sle.Limit != 1<<20 {
		t.Errorf("limit = %d", sle.Limit)
	}
}

func TestCapBufferBounds(t *testing.T) {
	c := &capBuffer{limit: 4}
	n, _ := c.Write([]byte("abcdefgh"))
	if n != 8 || c.String() != "abcd" {
		t.Errorf("n=%d buf=%q", n, c.String())
	}
}

func TestOptionsSandboxValidation(t *testing.T) {
	if _, err := New(document.Options{LatexSandbox: "bogus"}); !errors.Is(err, document.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestE2EBwrapBlocksAbsoluteInput(t *testing.T) {
	if _, err := exec.LookPath(document.DefaultLatexCommand); err != nil {
		t.Skip("pdflatex not installed")
	}
	d, err := New(document.Options{LatexSandbox: document.LatexSandboxRequired, LatexAllowUnsafeSource: true, Timeout: 2 * time.Minute})
	if errors.Is(err, ErrSandboxUnavailable) {
		t.Skip("bubblewrap unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	src := []byte(`\documentclass{article}\begin{document}\input{/etc/hostname}\end{document}`)
	if _, err := d.Render(t.Context(), src, document.FormatPDF); !errors.Is(err, ErrRenderFailed) {
		t.Fatalf("Render() err = %v, want ErrRenderFailed (file invisible in sandbox)", err)
	}
	ok := []byte(`\documentclass{article}\begin{document}hello\end{document}`)
	if _, err := d.Render(t.Context(), ok, document.FormatPDF); err != nil {
		t.Fatalf("benign render in sandbox failed: %v", err)
	}
}
