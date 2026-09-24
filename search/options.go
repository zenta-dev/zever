package search

import (
	"errors"
	"net/url"
)

// Options configures search backend selection and connection.
type Options struct {
	// Host holds the Meilisearch URL required by meilisearch.
	Host string `json:"host" toml:"host" yaml:"host"`
	// APIKey holds the backend API key, never logged.
	APIKey string `json:"api_key" toml:"api_key" yaml:"api_key"`
	// DSN holds the postgres URL or sqlite path, required by postgres/sqlite.
	DSN string `json:"dsn" toml:"dsn" yaml:"dsn"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Host != "" {
		u, err := url.Parse(o.Host)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, &InvalidOptionsError{Reason: "host must be a valid url with scheme and host"})
		}
	}

	return errors.Join(errs...)
}
