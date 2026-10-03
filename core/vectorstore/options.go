package vectorstore

import (
	"errors"

	"github.com/zenta-dev/zever/shared/endpoint"
)

const (
	// DefaultDimension is the default embedding dimension.
	DefaultDimension = 1536
	// DefaultTopK is the default number of query matches.
	DefaultTopK = 10
	// DefaultSQLiteDSN is the default SQLite data source name.
	DefaultSQLiteDSN = ":memory:"
)

// Options configures vectorstore backend selection and connection.
type Options struct {
	// DSN holds the sqlite path or postgres URL.
	DSN string `json:"dsn" toml:"dsn" yaml:"dsn"`
	// DedicatedPool opts out of container-level pool sharing. Default false
	// shares one pool per exact DSN; true opens a private pool.
	DedicatedPool bool `json:"dedicated_pool" toml:"dedicated_pool" yaml:"dedicated_pool"`
	// URL holds the Qdrant address.
	URL string `json:"url" toml:"url" yaml:"url"`
	// APIKey holds the backend credential and is never logged.
	APIKey string `json:"api_key" toml:"api_key" yaml:"api_key"`
	// Dimension holds the expected embedding dimension.
	Dimension int `json:"dimension" toml:"dimension" yaml:"dimension"`
	// AllowInsecure permits an http:// URL for testing. Default false (https only).
	AllowInsecure bool `json:"allow_insecure" toml:"allow_insecure" yaml:"allow_insecure"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Dimension < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "dimension must not be negative"})
	}

	if o.URL != "" {
		if _, err := endpoint.ValidateURL(o.URL, endpoint.WithAllowInsecure(o.AllowInsecure)); err != nil {
			switch {
			case errors.Is(err, endpoint.ErrParse):
				errs = append(errs, &InvalidOptionsError{Reason: "url must be a valid url"})
			case errors.Is(err, endpoint.ErrEmpty),
				errors.Is(err, endpoint.ErrNoScheme),
				errors.Is(err, endpoint.ErrNoHost):
				errs = append(errs, &InvalidOptionsError{Reason: "url must have scheme and host"})
			default:
				errs = append(errs, &InvalidOptionsError{Reason: "url must use https"})
			}
		}
	}

	return errors.Join(errs...)
}
