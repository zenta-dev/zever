package ai

import (
	"errors"
	"time"

	"github.com/zenta-dev/zever/internal/endpoint"
)

// Options configures the AI backend.
type Options struct {
	APIKey  string        `json:"api_key" toml:"api_key" yaml:"api_key"`
	Model   string        `json:"model" toml:"model" yaml:"model"`
	BaseURL string        `json:"base_url" toml:"base_url" yaml:"base_url"`
	Timeout time.Duration `json:"timeout" toml:"timeout" yaml:"timeout"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Timeout < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "timeout must be >= 0"})
	}

	if o.BaseURL != "" {
		if _, err := endpoint.ValidateURL(o.BaseURL); err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: baseURLReason(err)})
		}
	}

	return errors.Join(errs...)
}

// baseURLReason maps endpoint validation failures to the historical
// base_url reason strings.
func baseURLReason(err error) string {
	switch {
	case errors.Is(err, endpoint.ErrParse):
		return "base_url must be a valid url"
	case errors.Is(err, endpoint.ErrNoScheme):
		return "base_url must include scheme"
	case errors.Is(err, endpoint.ErrNoHost):
		return "base_url must include host"
	default:
		return "base_url must use https scheme"
	}
}
