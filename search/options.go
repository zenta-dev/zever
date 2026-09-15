package search

import (
	"errors"
	"net/url"
)

// Options configures search backend selection and connection.
type Options struct {
	// Host holds the Meilisearch URL required by meilisearch.
	Host string
	// APIKey holds the backend API key, never logged.
	APIKey string
	// DSN holds the postgres URL or sqlite path, required by postgres/sqlite.
	DSN string
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Host != "" {
		u, err := url.Parse(o.Host)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, &InvalidOptionsError{Reason: "host must be a valid URL with scheme and host"})
		}
	}

	return errors.Join(errs...)
}
