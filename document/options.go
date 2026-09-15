package document

import (
	"errors"
	"net/url"
	"time"
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
	TmpDir string
	// Timeout bounds document render operations.
	Timeout time.Duration
	// Quality controls raster output quality in the range 0-100.
	Quality int
	// DPI controls raster output resolution.
	DPI int
	// Endpoint holds the optional remote render endpoint URL.
	Endpoint string
	// APIKey holds the remote API key. It is never logged.
	APIKey string
	// LatexCommand holds the LaTeX compiler command.
	LatexCommand string
	// LatexPDFToPPM holds the PDF-to-image converter command.
	LatexPDFToPPM string
	// MaxOutputBytes bounds the rendered output size.
	MaxOutputBytes int64
	// LatexRuns bounds the number of LaTeX compiler passes.
	LatexRuns int
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
		errs = append(errs, &InvalidOptionsError{Reason: "max output bytes must be >= 0"})
	}

	if o.LatexRuns < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "latex runs must be >= 0"})
	}

	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid URL"})
		} else {
			if u.Scheme == "" {
				errs = append(errs, &InvalidOptionsError{Reason: "endpoint must include scheme"})
			}

			if u.Host == "" {
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
