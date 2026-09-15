package billing

import (
	"errors"
	"net/url"
	"time"
)

const (
	// DefaultHTTPTimeout is the default HTTP timeout for provider calls.
	DefaultHTTPTimeout = 30 * time.Second
)

// Options configures billing backend selection and limits.
type Options struct {
	// SecretKey holds the provider secret key. It is never logged.
	SecretKey string
	// APIKey holds the provider API key. It is never logged.
	APIKey string
	// Endpoint holds the optional custom provider endpoint URL.
	Endpoint string
	// Sandbox selects the provider sandbox environment.
	Sandbox bool
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid URL"})
		} else if u.Scheme == "" || u.Host == "" {
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid URL"})
		}
	}

	return errors.Join(errs...)
}
