package pretty

import (
	"errors"

	"github.com/zenta-dev/zever/internal/opts"
	"github.com/zenta-dev/zever/log"
)

// Options holds typed configuration for the pretty logger.
type Options struct {
	// Level selects the minimum severity emitted, as a canonical level
	// name ("debug", "info", "warn", "error", "fatal"). Empty selects
	// the info default.
	Level string `json:"level" toml:"level" yaml:"level"`
	// Color forces color output on (true) or off (false). Nil selects
	// automatic detection: color only when writing to a terminal with
	// NO_COLOR unset and TERM not "dumb".
	Color *bool `json:"color" toml:"color" yaml:"color"`
}

// ParseOptions extracts a typed Options from the raw option map, rejecting
// unknown keys and wrongly-typed values instead of silently ignoring them.
func ParseOptions(m map[string]any) (Options, error) {
	if m == nil {
		m = map[string]any{}
	}

	for k := range m {
		switch k {
		case "level", "color":
		default:
			return Options{}, &UnknownOptionError{Option: k}
		}
	}

	var o Options

	if v, ok := m["level"]; ok {
		s, err := opts.StrictString("pretty", "level", v)
		if err != nil {
			return Options{}, &InvalidOptionError{Option: "level"}
		}

		o.Level = s
	} else {
		o.Level = "info"
	}

	if v, ok := m["color"]; ok {
		b, err := opts.StrictBool("pretty", "color", v)
		if err != nil {
			return Options{}, &InvalidOptionError{Option: "color"}
		}

		o.Color = &b
	}

	return o, nil
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Level != "" {
		if _, err := log.ParseLevel(o.Level); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
