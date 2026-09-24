package geo

import (
	"errors"
	"time"

	"github.com/zenta-dev/zever/internal/endpoint"
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

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error
	if o.Timeout < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "timeout must be >= 0"})
	}
	if o.MaxResponseBody < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max_response_body must be >= 0"})
	}
	if o.BaseURL != "" {
		if _, err := endpoint.ValidateURL(o.BaseURL, endpoint.WithAllowInsecure(o.AllowInsecure)); err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: httpsReason("base_url", err)})
		}
	}
	if o.Endpoint != "" {
		if _, err := endpoint.ValidateURL(o.Endpoint, endpoint.WithAllowInsecure(o.AllowInsecure)); err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: httpsReason("endpoint", err)})
		}
	}
	return errors.Join(errs...)
}

// httpsReason maps endpoint validation failures to the historical geo
// reason strings for field: shape problems report a valid URL, scheme
// problems report the https requirement.
func httpsReason(field string, err error) string {
	switch {
	case errors.Is(err, endpoint.ErrParse),
		errors.Is(err, endpoint.ErrEmpty),
		errors.Is(err, endpoint.ErrNoScheme),
		errors.Is(err, endpoint.ErrNoHost):
		return field + " must be a valid url"
	default:
		return field + " must use https"
	}
}
