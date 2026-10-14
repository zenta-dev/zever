package latex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zenta-dev/zever/core/document"
)

const (
	jobName   = "doc"
	maxRuns   = 5
	maxStderr = 4096
	// ioWaitDelay bounds how long Wait lingers for child I/O pipes after the
	// process (or its whole group, via Cancel) is gone, so a pipe-holding
	// grandchild cannot stall Render after a kill.
	ioWaitDelay = 5 * time.Second
)

var _ document.Document = (*driver)(nil)

type driver struct {
	command     string
	converter   string
	runs        int
	dpi         int
	quality     int
	timeout     time.Duration
	tmpDir      string
	maxOutput   int64
	allowUnsafe bool
	sb          *sandbox
}

// New creates a LaTeX document backend from the given options.
func New(o document.Options) (document.Document, error) {
	return newDriver(o, defaultDeps())
}

func newDriver(o document.Options, dp deps) (*driver, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("latex: %w", err)
	}

	cmd := o.LatexCommand
	if cmd == "" {
		cmd = document.DefaultLatexCommand
	}
	resolved, err := exec.LookPath(cmd)
	if err != nil {
		return nil, fmt.Errorf("latex: command %q: %w", cmd, err)
	}

	// Converter stays unresolved so PDF-only users need no poppler install.
	converter := o.LatexPDFToPPM
	if converter == "" {
		converter = document.DefaultLatexConverter
	}

	runs := o.LatexRuns
	if runs <= 0 {
		runs = 1
	} else if runs > maxRuns {
		runs = maxRuns
	}

	dpi := o.DPI
	if dpi <= 0 {
		dpi = document.DefaultLatexDPI
	}

	quality := document.ClampQuality(o.Quality)
	if quality <= 0 {
		quality = document.DefaultLatexQuality
	}

	timeout := o.Timeout
	if timeout <= 0 {
		timeout = document.DefaultTimeout
	}

	maxOutput := o.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = document.DefaultMaxOutputBytes
	}

	sb, err := newSandbox(o.LatexSandbox, resolved, dp)
	if err != nil {
		return nil, err
	}

	return &driver{
		sb:          sb,
		allowUnsafe: o.LatexAllowUnsafeSource,
		command:     resolved,
		converter:   converter,
		runs:        runs,
		dpi:         dpi,
		quality:     quality,
		timeout:     timeout,
		tmpDir:      o.TmpDir,
		maxOutput:   maxOutput,
	}, nil
}

// Render compiles source into the requested format.
func (d *driver) Render(ctx context.Context, source []byte, format document.OutputFormat) ([]byte, error) {
	switch format {
	case document.FormatPDF, document.FormatPNG, document.FormatJPG:
	default:
		unsupported := error(document.UnsupportedFormatError{Format: format})
		return nil, fmt.Errorf("latex: %w", unsupported)
	}

	if len(source) > document.DefaultMaxSourceBytes {
		oversized := error(document.SizeLimitError{Size: len(source), Limit: document.DefaultMaxSourceBytes})
		return nil, fmt.Errorf("latex: %w", oversized)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("latex: render: %w", err)
	}

	if !d.allowUnsafe {
		if err := scanSource(source); err != nil {
			return nil, err
		}
	}

	dir, err := os.MkdirTemp(d.tmpDir, "latex-")
	if err != nil {
		return nil, fmt.Errorf("latex: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	if err := d.compile(ctx, dir, source); err != nil {
		return nil, err
	}

	pdfPath := filepath.Join(dir, jobName+".pdf")
	if format == document.FormatPDF {
		return d.readCapped(pdfPath, "output")
	}

	return d.convert(ctx, dir, pdfPath, format)
}

// newCmd builds an exec.Cmd for name, wrapped in the OS sandbox if active.
func (d *driver) newCmd(ctx context.Context, dir, name string, argv []string) *exec.Cmd {
	if d.sb != nil {
		name, argv = d.sb.wrap(dir, name, argv)
	}
	// binary from validated config resolved at Open; no shell; fixed argv.
	cmd := exec.CommandContext(ctx, name, argv...)
	cmd.Dir = dir
	cmd.Env = sandboxEnv(dir)
	withGroupKill(cmd)
	return cmd
}

func (d *driver) compile(ctx context.Context, dir string, source []byte) error {
	// -no-shell-escape is defense-in-depth against \write18/\immediate\write18
	// (arbitrary shell command execution from within the document) regardless
	// of what the host's texmf.cnf otherwise allows: source is caller-supplied
	// LaTeX (e.g. an invoice/report template with interpolated data), and this
	// package's generated documents have no legitimate need for shell-escape-
	// dependent packages (minted, epstopdf, svg, ...). sandboxEnv scrubs the
	// child environment (no parent secrets, HOME redirected into the render
	// dir) and pins kpathsea to paranoid openout_any=p (blocks \openout writes
	// outside the render tree) and openin_any=p (blocks \input outside the
	// render tree on TeX Live releases that still honor it: as of TeX Live
	// 2026, openin_any is a documented no-op upstream, see texmf.cnf).
	argv := []string{"-interaction=nonstopmode", "-halt-on-error", "-no-shell-escape", "-jobname=" + jobName}
	for i := 0; i < d.runs; i++ {
		cmd := d.newCmd(ctx, dir, d.command, argv)
		cmd.Stdin = bytes.NewReader(source)
		stderr := &capBuffer{limit: maxStderr}
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%w: %q", errors.Join(ErrRenderFailed, err), stderr.String())
		}
	}

	return nil
}

// capBuffer keeps at most limit bytes and silently discards the rest, so a
// hostile document cannot exhaust memory through stderr.
type capBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if room := c.limit - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (c *capBuffer) String() string { return c.buf.String() }

// readCapped reads path without ever buffering more than maxOutput bytes: a
// stat check rejects oversized files up front, and a LimitReader guards
// against the file growing between stat and read.
func (d *driver) readCapped(path, what string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("latex: read %s: %w", what, err)
	}
	defer func() { _ = f.Close() }()

	oversized := func(size int64) error {
		return fmt.Errorf("latex: %w", error(document.SizeLimitError{Size: int(min(size, int64(^uint(0)>>1))), Limit: int(d.maxOutput)}))
	}
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("latex: read %s: %w", what, err)
	}
	if st.Size() > d.maxOutput {
		return nil, oversized(st.Size())
	}
	b, err := io.ReadAll(io.LimitReader(f, d.maxOutput+1))
	if err != nil {
		return nil, fmt.Errorf("latex: read %s: %w", what, err)
	}
	if int64(len(b)) > d.maxOutput {
		return nil, oversized(int64(len(b)))
	}

	return b, nil
}

// sandboxEnv builds the minimal environment for a child process: PATH (so
// TeX can run its helper binaries), HOME redirected to the render directory
// (so ~-relative paths resolve inside the sandbox), and any operator-set
// TEXMF* variables (system font/package trees). openin_any=p requests
// paranoid \input handling (a no-op on TeX Live >= 2026, which still protects
// older releases), and openout_any=p restricts \openout writes to the render
// tree. Every other inherited variable (credentials, proxy settings,
// XAUTHORITY, ...) is dropped.
func sandboxEnv(dir string) []string {
	env := []string{"HOME=" + dir, "openin_any=p", "openout_any=p"}
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "TEXMF") {
			env = append(env, kv)
		}
	}
	return env
}

func (d *driver) convert(ctx context.Context, dir, pdfPath string, format document.OutputFormat) ([]byte, error) {
	conv, lookErr := exec.LookPath(d.converter)
	if lookErr != nil {
		return nil, ErrMissingConverter
	}

	// Local poppler supports -singlefile, so output lands at root+ext with no page digits.
	root := filepath.Join(dir, "out")
	dpi := strconv.Itoa(d.dpi)
	var argv []string
	ext := ".png"
	if format == document.FormatPNG {
		argv = []string{"-png", "-r", dpi, "-singlefile", pdfPath, root}
	} else {
		argv = []string{"-jpeg", "-jpegopt", "quality=" + strconv.Itoa(d.quality), "-r", dpi, "-singlefile", pdfPath, root}
		ext = ".jpg"
	}

	// binary resolved via LookPath; no shell; numeric-only variable argv.
	cmd := d.newCmd(ctx, dir, conv, argv)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("latex: %w", errors.Join(ErrRenderFailed, err))
	}

	return d.readCapped(root+ext, "image")
}

// Close releases LaTeX backend resources.
func (d *driver) Close() error {
	return nil
}
