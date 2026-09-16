package ai

import (
	"errors"
	"net/url"
	"time"
)

// Options configures the AI backend.
type Options struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Timeout < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "timeout must be >= 0"})
	}

	if o.BaseURL != "" {
		u, err := url.Parse(o.BaseURL)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "base_url must be a valid URL"})
		} else {
			if u.Scheme == "" {
				errs = append(errs, &InvalidOptionsError{Reason: "base_url must include scheme"})
			}
			if u.Host == "" {
				errs = append(errs, &InvalidOptionsError{Reason: "base_url must include host"})
			}
			if u.Scheme != "" && u.Scheme != "https" {
				errs = append(errs, &InvalidOptionsError{Reason: "base_url must use https scheme"})
			}
		}
	}

	return errors.Join(errs...)
}
