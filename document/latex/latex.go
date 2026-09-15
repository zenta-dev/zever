package latex

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zenta-dev/zever/document"
)

const (
	jobName   = "doc"
	maxRuns   = 5
	maxStderr = 4096
)

var _ document.Document = (*driver)(nil)

type driver struct {
	command   string
	converter string
	runs      int
	dpi       int
	quality   int
	timeout   time.Duration
	tmpDir    string
	maxOutput int64
}

// Open creates a LaTeX document backend from the given options.
func Open(o document.Options) (document.Document, error) {
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

	return &driver{
		command:   resolved,
		converter: converter,
		runs:      runs,
		dpi:       dpi,
		quality:   quality,
		timeout:   timeout,
		tmpDir:    o.TmpDir,
		maxOutput: maxOutput,
	}, nil
}

// Render compiles source into the requested format.
func (d *driver) Render(ctx context.Context, source []byte, format document.OutputFormat) ([]byte, error) {
	switch format {
	case document.FormatPDF, document.FormatPNG, document.FormatJPG:
	default:
		unsupported := error(&document.UnsupportedFormatError{Format: format})
		return nil, fmt.Errorf("latex: %w", unsupported)
	}

	if len(source) > document.DefaultMaxSourceBytes {
		oversized := error(&document.SizeLimitError{Size: len(source), Limit: document.DefaultMaxSourceBytes})
		return nil, fmt.Errorf("latex: %w", oversized)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("latex: render: %w", err)
	}

	dir, err := os.MkdirTemp(d.tmpDir, "latex-")
	if err != nil {
		return nil, fmt.Errorf("latex: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	pdf, err := d.compile(ctx, dir, source)
	if err != nil {
		return nil, err
	}

	if format == document.FormatPDF {
		return d.cap(pdf)
	}

	return d.convert(ctx, dir, pdf, format)
}

func (d *driver) compile(ctx context.Context, dir string, source []byte) ([]byte, error) {
	argv := []string{"-interaction=nonstopmode", "-halt-on-error", "-jobname=" + jobName}
	for i := 0; i < d.runs; i++ {
		//nolint:gosec // binary from validated config resolved at Open; no shell; fixed argv.
		cmd := exec.CommandContext(ctx, d.command, argv...)
		cmd.Dir = dir
		cmd.Stdin = bytes.NewReader(source)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			out := stderr.String()
			return nil, fmt.Errorf("%w: %w: %q", ErrRenderFailed, err, out[:min(len(out), maxStderr)])
		}
	}

	pdf, err := os.ReadFile(filepath.Join(dir, jobName+".pdf"))
	if err != nil {
		return nil, fmt.Errorf("latex: read output: %w", err)
	}

	return pdf, nil
}

func (d *driver) cap(b []byte) ([]byte, error) {
	if int64(len(b)) > d.maxOutput {
		oversized := error(&document.SizeLimitError{Size: len(b), Limit: int(d.maxOutput)})
		return nil, fmt.Errorf("latex: %w", oversized)
	}

	return b, nil
}

func (d *driver) convert(ctx context.Context, dir string, pdf []byte, format document.OutputFormat) ([]byte, error) {
	conv, lookErr := exec.LookPath(d.converter)
	if lookErr != nil {
		return nil, ErrMissingConverter
	}

	pdfPath := filepath.Join(dir, jobName+".pdf")
	if err := os.WriteFile(pdfPath, pdf, 0o600); err != nil {
		return nil, fmt.Errorf("latex: write pdf: %w", err)
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

	//nolint:gosec // binary resolved via LookPath; no shell; numeric-only variable argv.
	cmd := exec.CommandContext(ctx, conv, argv...)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRenderFailed, err)
	}

	img, err := os.ReadFile(root + ext)
	if err != nil {
		return nil, fmt.Errorf("latex: read image: %w", err)
	}

	return d.cap(img)
}

// Close releases LaTeX backend resources.
func (d *driver) Close() error {
	return nil
}
