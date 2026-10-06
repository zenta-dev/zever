package cdn

import (
	"errors"
	"fmt"
	"strings"
)

// Options carries the settings for a CDN backend.
type Options struct {
	// APIToken authenticates against the provider API.
	APIToken string `json:"api_token" toml:"api_token" yaml:"api_token"`
	// ZoneID identifies the zone or distribution to purge.
	ZoneID string `json:"zone_id" toml:"zone_id" yaml:"zone_id"`
	// BaseURL overrides the provider API endpoint.
	BaseURL string `json:"base_url" toml:"base_url" yaml:"base_url"`
}

// Validate checks Options fail-closed and joins all violations. Zero values
// pass: config.Default must validate clean, so credential presence is
// enforced by each adapter's New instead.
func (o Options) Validate() error {
	var errs []error
	if o.BaseURL != "" && !strings.HasPrefix(o.BaseURL, "https://") && !strings.HasPrefix(o.BaseURL, "http://") {
		errs = append(errs, fmt.Errorf("cdn: option %q must be a valid URL", "base_url"))
	}
	return errors.Join(errs...)
}
