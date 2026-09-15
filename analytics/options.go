package analytics

import (
	"errors"
	"net/url"
)

const (
	// DefaultAnonymousID is the fallback identity for anonymous tracking.
	DefaultAnonymousID = "anonymous"
	// DefaultGroupType is the fallback group type for group calls.
	DefaultGroupType = "organization"
	// DefaultMaxPropertiesBytes is the default byte limit for properties payloads.
	DefaultMaxPropertiesBytes = 64 * 1024
)

// Options configures analytics backend selection and limits.
type Options struct {
	// APIKey holds the backend write key. It is never logged.
	APIKey string
	// Endpoint holds the optional custom backend endpoint URL.
	Endpoint string
	// AnonymousID holds the fallback identity for anonymous tracking.
	AnonymousID string
	// GroupType holds the group type for group calls.
	GroupType string
	// MaxPropertiesBytes bounds the JSON-encoded properties payload size.
	MaxPropertiesBytes int
	// MaxProperties bounds the number of properties per call.
	MaxProperties int
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.MaxPropertiesBytes < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max properties bytes must be >= 0"})
	}

	if o.MaxProperties < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max properties must be >= 0"})
	}

	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid URL"})
		} else {
			if u.Scheme == "" {
				errs = append(errs, &InvalidOptionsError{Reason: "endpoint must include scheme"})
			}

			if u.Host == "" {
				errs = append(errs, &InvalidOptionsError{Reason: "endpoint must include host"})
			}
		}
	}

	return errors.Join(errs...)
}

// anonymousID returns AnonymousID or DefaultAnonymousID when unset.
func (o Options) anonymousID() string {
	if o.AnonymousID == "" {
		return DefaultAnonymousID
	}

	return o.AnonymousID
}

// groupType returns GroupType or DefaultGroupType when unset.
func (o Options) groupType() string {
	if o.GroupType == "" {
		return DefaultGroupType
	}

	return o.GroupType
}

// maxPropertiesBytes returns MaxPropertiesBytes or DefaultMaxPropertiesBytes when unset.
func (o Options) maxPropertiesBytes() int {
	if o.MaxPropertiesBytes <= 0 {
		return DefaultMaxPropertiesBytes
	}

	return o.MaxPropertiesBytes
}
