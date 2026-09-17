// Package opts provides shared helpers for reading typed values out of the
// raw `map[string]any` option maps that adapters are configured with.
package opts

import (
	"fmt"
	"time"
)

// String extracts a string option from the map. Returns def if the key is
// missing or the value is not a string.
func String(m map[string]any, key string, def string) string {
	if m == nil {
		return def
	}

	v, ok := m[key]
	if !ok {
		return def
	}

	s, ok := v.(string)
	if !ok {
		return def
	}

	return s
}

// RequireString extracts a required string option. Returns an error if the
// key is missing, empty, or not a string.
func RequireString(m map[string]any, key string) (string, error) {
	s := String(m, key, "")
	if s == "" {
		return "", fmt.Errorf("option %q is required", key)
	}

	return s, nil
}

// Int extracts an int option, accepting int, int64, and float64 values.
// Returns def if the key is missing or the value is not a numeric type.
func Int(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}

	v, ok := m[key]
	if !ok {
		return def
	}

	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return def
	}
}

// Int64 extracts an int64 option, accepting int, int64, and float64 values.
// Returns def if the key is missing or the value is not a numeric type.
func Int64(m map[string]any, key string, def int64) int64 {
	if m == nil {
		return def
	}

	v, ok := m[key]
	if !ok {
		return def
	}

	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return def
	}
}

// Float64 extracts a float64 option, accepting int, int64, and float64 values.
// Returns def if the key is missing or the value is not a numeric type.
func Float64(m map[string]any, key string, def float64) float64 {
	if m == nil {
		return def
	}

	v, ok := m[key]
	if !ok {
		return def
	}

	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return def
	}
}

// Bool extracts a bool option. Returns def if the key is missing or the
// value is not a bool. Numeric 1/0 (int, int64, float64) are also accepted
// so that env var coerce of "1"/"0" as int does not silently fall back to
// def; non-zero is true.
func Bool(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}

	v, ok := m[key]
	if !ok {
		return def
	}

	switch b := v.(type) {
	case bool:
		return b
	case int:
		return b != 0
	case int64:
		return b != 0
	case float64:
		return b != 0
	case string:
		switch b {
		case "true", "True", "TRUE", "1", "t", "T", "yes", "Yes", "YES", "on", "On", "ON":
			return true
		case "false", "False", "FALSE", "0", "f", "F", "no", "No", "NO", "off", "Off", "OFF":
			return false
		}
	}

	return def
}

// Duration extracts a time.Duration option. Accepts time.Duration, numeric
// types (interpreted as seconds), and string values (parsed via
// time.ParseDuration). Returns def if the key is missing or the value
// cannot be interpreted as a duration.
func Duration(m map[string]any, key string, def time.Duration) time.Duration {
	if m == nil {
		return def
	}

	v, ok := m[key]
	if !ok {
		return def
	}

	switch d := v.(type) {
	case time.Duration:
		return d
	case int:
		return time.Duration(d) * time.Second
	case int64:
		return time.Duration(d) * time.Second
	case float64:
		return time.Duration(float64(time.Second) * d)
	case string:
		if parsed, err := time.ParseDuration(d); err == nil {
			return parsed
		}
	}

	return def
}

// StringSlice extracts a []string option. Accepts []string and []any
// (where each element is a string). Returns nil if the key is missing or
// the value is not a recognized slice type.
func StringSlice(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}

	v, ok := m[key]
	if !ok {
		return nil
	}

	switch sl := v.(type) {
	case []string:
		return sl
	case []any:
		result := make([]string, 0, len(sl))
		for _, item := range sl {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}

		return result
	}

	return nil
}

// Map extracts a map[string]any option. Returns nil if the key is missing or
// the value is not a map[string]any.
func Map(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}

	v, ok := m[key]
	if !ok {
		return nil
	}

	if result, ok := v.(map[string]any); ok {
		return result
	}

	return nil
}

// StrictString type-checks v (a raw option value already looked up under
// key) as a string, for use by strict ParseOptions implementations that
// reject wrongly-typed values instead of silently falling back to a
// default. pkgTag is the bracketed error-message tag (e.g. "cache").
func StrictString(pkgTag, key string, v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("[%s] option %q must be a string, got %T", pkgTag, key, v)
	}

	return s, nil
}

// StrictBool type-checks v as a bool, for strict ParseOptions
// implementations.
func StrictBool(pkgTag, key string, v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("[%s] option %q must be a bool, got %T", pkgTag, key, v)
	}

	return b, nil
}

// StrictInt type-checks v as an integer (accepting int, int64, and float64),
// for strict ParseOptions implementations.
func StrictInt(pkgTag, key string, v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("[%s] option %q must be an integer, got %T", pkgTag, key, v)
	}
}

// StrictInt64 type-checks v as an int64 (accepting int, int64, and
// float64), for strict ParseOptions implementations.
func StrictInt64(pkgTag, key string, v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case float64:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("[%s] option %q must be an integer, got %T", pkgTag, key, v)
	}
}

// StrictFloat64 type-checks v as a float64 (accepting int, int64, and
// float64), for strict ParseOptions implementations.
func StrictFloat64(pkgTag, key string, v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case int64:
		return float64(n), nil
	case int:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("[%s] option %q must be a number, got %T", pkgTag, key, v)
	}
}

// StrictDuration type-checks v as a time.Duration, accepting a
// time.Duration, numeric types (interpreted as seconds), or a string
// (parsed via time.ParseDuration), for strict ParseOptions implementations.
func StrictDuration(pkgTag, key string, v any) (time.Duration, error) {
	switch d := v.(type) {
	case time.Duration:
		return d, nil
	case int:
		return time.Duration(d) * time.Second, nil
	case int64:
		return time.Duration(d) * time.Second, nil
	case float64:
		return time.Duration(d * float64(time.Second)), nil
	case string:
		parsed, err := time.ParseDuration(d)
		if err != nil {
			return 0, fmt.Errorf("[%s] option %q must be a duration: %w", pkgTag, key, err)
		}

		return parsed, nil
	default:
		return 0, fmt.Errorf("[%s] option %q must be a duration, got %T", pkgTag, key, v)
	}
}

// StrictStringSlice type-checks v as a []string, accepting []string and
// []any (where every element must be a string), for strict ParseOptions
// implementations.
func StrictStringSlice(pkgTag, key string, v any) ([]string, error) {
	switch sl := v.(type) {
	case []string:
		return sl, nil
	case []any:
		result := make([]string, 0, len(sl))

		for _, item := range sl {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("[%s] option %q must be a list of strings, got element of type %T", pkgTag, key, item)
			}

			result = append(result, s)
		}

		return result, nil
	default:
		return nil, fmt.Errorf("[%s] option %q must be a list of strings, got %T", pkgTag, key, v)
	}
}

// StrictMap type-checks v as a map[string]any, for strict ParseOptions
// implementations.
func StrictMap(pkgTag, key string, v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("[%s] option %q must be a map, got %T", pkgTag, key, v)
	}

	return m, nil
}
