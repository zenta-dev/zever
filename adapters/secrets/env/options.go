package env

import (
	"errors"
	"strings"
)

// Options holds typed configuration for the env secrets adapter.
type Options struct {
	// Prefix scopes secret names to one environment variable namespace. Required.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
}

// Validate checks Options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Prefix == "" {
		errs = append(errs, errors.New("env: prefix is required"))
	}

	if strings.Contains(o.Prefix, "/") || strings.Contains(o.Prefix, "..") {
		errs = append(errs, errors.New("env: prefix must not contain slash or dot-dot"))
	}

	if strings.Contains(o.Prefix, " ") {
		errs = append(errs, errors.New("env: prefix must not contain spaces"))
	}

	return errors.Join(errs...)
}
