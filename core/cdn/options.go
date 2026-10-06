package cdn

import (
	"errors"
	"fmt"
	"strings"
)

type Options struct {
	APIToken string `json:"api_token" toml:"api_token" yaml:"api_token"`
	ZoneID   string `json:"zone_id" toml:"zone_id" yaml:"zone_id"`
	BaseURL  string `json:"base_url" toml:"base_url" yaml:"base_url"`
}

func (o Options) Validate() error {
	var errs []error
	if o.APIToken == "" {
		errs = append(errs, fmt.Errorf("cdn: option %q is required", "api_token"))
	}
	if o.ZoneID == "" {
		errs = append(errs, fmt.Errorf("cdn: option %q is required", "zone_id"))
	}
	if o.BaseURL != "" && !strings.HasPrefix(o.BaseURL, "https://") && !strings.HasPrefix(o.BaseURL, "http://") {
		errs = append(errs, fmt.Errorf("cdn: option %q must be a valid URL", "base_url"))
	}
	return errors.Join(errs...)
}
