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
	SecretKey string `json:"secret_key" toml:"secret_key" yaml:"secret_key"`
	// APIKey holds the provider API key. It is never logged.
	APIKey string `json:"api_key" toml:"api_key" yaml:"api_key"`
	// Endpoint holds the optional custom provider endpoint URL.
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// Sandbox selects the provider sandbox environment.
	Sandbox bool `json:"sandbox" toml:"sandbox" yaml:"sandbox"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid url"})
		} else if u.Scheme == "" || u.Host == "" {
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid url"})
		}
	}

	return errors.Join(errs...)
}
