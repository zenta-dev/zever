package document

import (
	"errors"
	"time"

	"github.com/zenta-dev/zever/internal/endpoint"
)

const (
	// DefaultTimeout is the default timeout for document operations.
	DefaultTimeout = 30 * time.Second
	// DefaultMaxSourceBytes is the default byte limit for document sources.
	DefaultMaxSourceBytes = 10 << 20
	// DefaultMaxOutputBytes is the default byte limit for rendered output.
	DefaultMaxOutputBytes = 64 << 20
	// DefaultLatexDPI is the default DPI for LaTeX raster output.
	DefaultLatexDPI = 150
	// DefaultLatexQuality is the default quality for LaTeX raster output.
	DefaultLatexQuality = 80
	// DefaultLatexCommand is the default LaTeX compiler command.
	DefaultLatexCommand = "pdflatex"
	// DefaultLatexConverter is the default PDF-to-image converter command.
	DefaultLatexConverter = "pdftoppm"
)

// Options configures document backend selection and limits.
type Options struct {
	// TmpDir holds the directory for intermediate render artifacts.
	TmpDir string `json:"tmp_dir" toml:"tmp_dir" yaml:"tmp_dir"`
	// Timeout bounds document render operations.
	Timeout time.Duration `json:"timeout" toml:"timeout" yaml:"timeout"`
	// Quality controls raster output quality in the range 0-100.
	Quality int `json:"quality" toml:"quality" yaml:"quality"`
	// DPI controls raster output resolution.
	DPI int `json:"dpi" toml:"dpi" yaml:"dpi"`
	// Endpoint holds the optional remote render endpoint URL.
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// APIKey holds the remote API key. It is never logged.
	APIKey string `json:"api_key" toml:"api_key" yaml:"api_key"`
	// LatexCommand holds the LaTeX compiler command.
	LatexCommand string `json:"latex_command" toml:"latex_command" yaml:"latex_command"`
	// LatexPDFToPPM holds the PDF-to-image converter command.
	LatexPDFToPPM string `json:"latex_pdf_to_ppm" toml:"latex_pdf_to_ppm" yaml:"latex_pdf_to_ppm"`
	// MaxOutputBytes bounds the rendered output size.
	MaxOutputBytes int64 `json:"max_output_bytes" toml:"max_output_bytes" yaml:"max_output_bytes"`
	// LatexRuns bounds the number of LaTeX compiler passes.
	LatexRuns int `json:"latex_runs" toml:"latex_runs" yaml:"latex_runs"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Timeout < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "timeout must be >= 0"})
	}

	if o.Quality < 0 || o.Quality > 100 {
		errs = append(errs, &InvalidOptionsError{Reason: "quality must be between 0 and 100"})
	}

	if o.DPI < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "dpi must be >= 0"})
	}

	if o.MaxOutputBytes < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max_output_bytes must be >= 0"})
	}

	if o.LatexRuns < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "latex_runs must be >= 0"})
	}

	if o.Endpoint != "" {
		if _, err := endpoint.ValidateURL(o.Endpoint, endpoint.WithAllowAnyScheme()); err != nil {
			switch {
			case errors.Is(err, endpoint.ErrParse):
				errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid url"})
			case errors.Is(err, endpoint.ErrNoScheme):
				errs = append(errs, &InvalidOptionsError{Reason: "endpoint must include scheme"})
			default:
				errs = append(errs, &InvalidOptionsError{Reason: "endpoint must include host"})
			}
		}
	}

	return errors.Join(errs...)
}

// ClampQuality clamps q into the range 0-100.
func ClampQuality(q int) int {
	if q < 0 {
		return 0
	}

	if q > 100 {
		return 100
	}

	return q
}
