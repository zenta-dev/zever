package geo

import (
	"errors"
	"net/url"
	"time"
)

// Options holds typed configuration for geo adapters.
type Options struct {
	// APIKey is the provider API key. Never logged.
	APIKey string `json:"api_key" toml:"api_key" yaml:"api_key"`
	// BaseURL overrides the Google Maps API base URL.
	BaseURL string `json:"base_url" toml:"base_url" yaml:"base_url"`
	// Endpoint overrides the Mapbox/OSM API endpoint.
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// Timeout bounds HTTP requests. Zero means no timeout (adapter may apply default).
	Timeout time.Duration `json:"timeout" toml:"timeout" yaml:"timeout"`
	// Path is the JSON file for the static adapter.
	Path string `json:"path" toml:"path" yaml:"path"`
	// MaxResponseBody bounds response bodies. Zero means default.
	MaxResponseBody int `json:"max_response_body" toml:"max_response_body" yaml:"max_response_body"`
	// AllowInsecure permits http BaseURL/Endpoint for testing.
	AllowInsecure bool `json:"allow_insecure" toml:"allow_insecure" yaml:"allow_insecure"`
	// UserAgent is the User-Agent for OSM Nominatim. Required for OSM.
	UserAgent string `json:"user_agent" toml:"user_agent" yaml:"user_agent"`
}

// Validate checks Options for logical correctness.
func (o Options) Validate() error {
	var errs []error
	if o.Timeout < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "timeout must be >= 0"})
	}
	if o.MaxResponseBody < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max response body must be >= 0"})
	}
	if o.BaseURL != "" {
		u, err := url.Parse(o.BaseURL)
		switch {
		case err != nil || u.Scheme == "" || u.Host == "":
			errs = append(errs, &InvalidOptionsError{Reason: "base_url must be a valid URL"})
		case u.Scheme == "http" && !o.AllowInsecure:
			errs = append(errs, &InvalidOptionsError{Reason: "base_url must use https"})
		case u.Scheme != "https" && u.Scheme != "http":
			errs = append(errs, &InvalidOptionsError{Reason: "base_url must use https"})
		}
	}
	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		switch {
		case err != nil || u.Scheme == "" || u.Host == "":
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid URL"})
		case u.Scheme == "http" && !o.AllowInsecure:
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must use https"})
		case u.Scheme != "https" && u.Scheme != "http":
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must use https"})
		}
	}
	return errors.Join(errs...)
}
