package vectorstore

import (
	"errors"
	"net/url"
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
	DSN string
	// URL holds the Qdrant address.
	URL string
	// APIKey holds the backend credential and is never logged.
	APIKey string
	// Dimension holds the expected embedding dimension.
	Dimension int
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Dimension < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "dimension must not be negative"})
	}

	if o.URL != "" {
		u, err := url.Parse(o.URL)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "url must be a valid URL"})
		} else if u.Scheme == "" || u.Host == "" {
			errs = append(errs, &InvalidOptionsError{Reason: "url must have scheme and host"})
		}
	}

	return errors.Join(errs...)
}
