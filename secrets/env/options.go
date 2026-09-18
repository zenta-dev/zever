package env

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/internal/opts"
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

// ParseOptions extracts Options from a raw option map.
func ParseOptions(m map[string]any) (Options, error) {
	var prefix string

	if v, ok := m["prefix"]; ok && v != nil {
		s, err := opts.StrictString("env", "prefix", v)
		if err != nil {
			return Options{}, fmt.Errorf("env: option %q must be a string, got %T", "prefix", v)
		}

		prefix = s
	}

	opts := Options{Prefix: prefix}

	if err := opts.Validate(); err != nil {
		return Options{}, err
	}

	return opts, nil
}
