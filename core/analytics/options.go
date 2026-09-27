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
	APIKey string `json:"api_key" toml:"api_key" yaml:"api_key"`
	// Endpoint holds the optional custom backend endpoint URL.
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// AnonymousID holds the fallback identity for anonymous tracking.
	AnonymousID string `json:"anonymous_id" toml:"anonymous_id" yaml:"anonymous_id"`
	// GroupType holds the group type for group calls.
	GroupType string `json:"group_type" toml:"group_type" yaml:"group_type"`
	// MaxPropertiesBytes bounds the JSON-encoded properties payload size.
	MaxPropertiesBytes int `json:"max_properties_bytes" toml:"max_properties_bytes" yaml:"max_properties_bytes"`
	// MaxProperties bounds the number of properties per call.
	MaxProperties int `json:"max_properties" toml:"max_properties" yaml:"max_properties"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.MaxPropertiesBytes < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max_properties_bytes must be >= 0"})
	}

	if o.MaxProperties < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max_properties must be >= 0"})
	}

	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid url"})
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
