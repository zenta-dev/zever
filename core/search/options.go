package search

import (
	"errors"

	"github.com/zenta-dev/zever/shared/endpoint"
)

// Options configures search backend selection and connection.
type Options struct {
	// Host holds the Meilisearch URL required by meilisearch.
	Host string `json:"host" toml:"host" yaml:"host"`
	// APIKey holds the backend API key, never logged.
	APIKey string `json:"api_key" toml:"api_key" yaml:"api_key"`
	// DSN holds the postgres URL or sqlite path, required by postgres/sqlite.
	DSN string `json:"dsn" toml:"dsn" yaml:"dsn"`
	// DedicatedPool opts out of container-level pool sharing. Default false
	// shares one pool per exact DSN; true opens a private pool.
	DedicatedPool bool `json:"dedicated_pool" toml:"dedicated_pool" yaml:"dedicated_pool"`
	// AllowInsecure permits an http:// Host for testing. Default false (https only).
	AllowInsecure bool `json:"allow_insecure" toml:"allow_insecure" yaml:"allow_insecure"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Host != "" {
		if _, err := endpoint.ValidateURL(o.Host, endpoint.WithAllowInsecure(o.AllowInsecure)); err != nil {
			errs = append(errs, InvalidOptionsError{Reason: hostReason(err)})
		}
	}

	return errors.Join(errs...)
}

// hostReason maps endpoint validation failures to the historical search
// reason strings: field-shape problems report a valid URL, scheme problems
// report the https requirement.
func hostReason(err error) string {
	switch {
	case errors.Is(err, endpoint.ErrParse),
		errors.Is(err, endpoint.ErrEmpty),
		errors.Is(err, endpoint.ErrNoScheme),
		errors.Is(err, endpoint.ErrNoHost):
		return "host must be a valid url with scheme and host"
	default:
		return "host must use https"
	}
}
