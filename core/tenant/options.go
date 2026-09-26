package tenant

import (
	"errors"
	"strings"
)

const (
	// DefaultHeader is the default header carrying the tenant ID.
	DefaultHeader = "X-Tenant-ID"
	// DefaultSingleID is the default tenant ID for the single-tenant backend.
	DefaultSingleID = "default"
	// MaxRegexLength is the maximum allowed subdomain regex length.
	MaxRegexLength = 500
)

// Options configures tenant backend selection and resolution.
type Options struct {
	// Header holds the header name carrying the tenant ID.
	Header string `json:"header" toml:"header" yaml:"header"`
	// SubdomainRegex holds the optional subdomain matching pattern.
	SubdomainRegex string `json:"subdomain_regex" toml:"subdomain_regex" yaml:"subdomain_regex"`
	// ID holds the fixed tenant ID for the single-tenant backend.
	ID string `json:"id" toml:"id" yaml:"id"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Header != "" && !validHeaderKey(o.Header) {
		errs = append(errs, &InvalidOptionsError{Reason: "header must be a valid header key"})
	}

	if len(o.SubdomainRegex) > MaxRegexLength {
		errs = append(errs, &InvalidOptionsError{Reason: "subdomain_regex exceeds max length"})
	}

	return errors.Join(errs...)
}

// validHeaderKey reports whether s is a valid header key: printable ASCII
// without whitespace, colon, or DEL. Empty is invalid here; Validate treats
// "" as unset and lets adapters apply their default.
func validHeaderKey(s string) bool {
	if s == "" {
		return false
	}

	return !strings.ContainsFunc(s, func(r rune) bool {
		return r < '!' || r > '~' || r == ':'
	})
}
