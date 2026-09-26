package router

import (
	"errors"
	"unicode"

	"github.com/zenta-dev/zever/core/log"
)

// Options holds typed configuration for router adapters.
type Options struct {
	// AppName is the application name used by adapters, defaulting to "app" when empty.
	AppName string `json:"app_name" toml:"app_name" yaml:"app_name"`
	// Logger emits skipped-route warnings. Defaults to a no-op logger when nil.
	Logger log.Logger `json:"-" toml:"-" yaml:"-"`
}

// Validate checks options for consistency, joining all violations.
//
// Validate checks Options for logical correctness, joining all violations.
// An empty AppName is valid and lets the adapter default to "app".
func (o Options) Validate() error {
	if o.AppName == "" {
		return nil
	}

	var errs []error

	if len(o.AppName) > 64 {
		errs = append(errs, &InvalidOptionsError{Reason: "app_name must be at most 64 characters"})
	}

	for _, r := range o.AppName {
		if unicode.IsControl(r) {
			errs = append(errs, &InvalidOptionsError{Reason: "app_name must not contain control characters"})
			break
		}
	}

	return errors.Join(errs...)
}
