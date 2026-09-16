package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ServiceConfig is the raw file envelope for a single service: the adapter
// name plus its untyped option map. Name validation is not done here;
// unknown top-level service names pass through and merge rejects them.
type ServiceConfig struct {
	// Adapter names the backend implementation (for example "sqlite").
	Adapter string `json:"adapter" yaml:"adapter"`
	// Options carries the backend's untyped settings, decoded strictly
	// per service by decodeOptions.
	Options map[string]any `json:"options" yaml:"options"`
}

// decodeFile reads path and decodes the service envelopes it contains.
// Only YAML (.yaml, .yml) and JSON (.json) are supported; any other
// extension fails with ErrUnsupportedFormat. The extension match is
// case-insensitive.
//
// The top level must map service names to {adapter, options} blocks.
// Keys inside a service block are strict: anything besides adapter and
// options fails with a DecodeError naming the service. A null or empty
// block decodes to the zero ServiceConfig.
func decodeFile(path string) (map[string]ServiceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}

	raw := make(map[string]any)
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("config: parse yaml %q: %w: %w", path, ErrDecode, err)
		}
	case ".json":
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("config: parse json %q: %w: %w", path, ErrDecode, err)
		}
	default:
		return nil, fmt.Errorf("config: %q: unsupported format %q: %w", path, ext, ErrUnsupportedFormat)
	}

	out := make(map[string]ServiceConfig, len(raw))
	for name, entry := range raw {
		sc, err := decodeServiceEntry(name, entry)
		if err != nil {
			return nil, err
		}
		out[name] = sc
	}
	return out, nil
}

// decodeServiceEntry strictly decodes one service's envelope. The entry
// round-trips through JSON so YAML and JSON files share one strict path:
// unknown keys inside the block are rejected, and YAML scalars keep their
// types (ints stay ints, duration strings stay strings) until the typed
// decode reports them.
func decodeServiceEntry(name string, entry any) (ServiceConfig, error) {
	var sc ServiceConfig
	if entry == nil {
		return sc, nil
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return sc, &DecodeError{Service: name, Err: err}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sc); err != nil {
		return sc, &DecodeError{Service: name, Err: classifyDecodeError(name, err)}
	}
	return sc, nil
}

// decodeOptions strictly decodes a service's untyped option map into the
// service's typed options struct T. A nil or empty map yields the zero T
// with no error. Unknown fields are rejected, and type mismatches fail
// without echoing the offending value (see scrubTypeError).
//
// Matching follows encoding/json with DisallowUnknownFields: an exact key
// match wins, otherwise a case-insensitive match applies. Case folding
// ignores case only, so "maxconns" matches MaxConns but "max_conns" does
// not (underscores are significant). Duplicate keys are last-wins for
// JSON input but a syntax error for YAML input.
func decodeOptions[T any](service string, m map[string]any) (T, error) {
	var zero T
	if len(m) == 0 {
		return zero, nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return zero, &DecodeError{Service: service, Err: err}
	}
	var out T
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return zero, &DecodeError{Service: service, Err: classifyDecodeError(service, err)}
	}
	return out, nil
}

// classifyDecodeError maps a strict-decode failure to a safe error: type
// mismatches are value-scrubbed, unknown fields become UnknownFieldError
// so callers can match ErrUnknownField, and anything else passes through
// (decoder syntax and unknown-field errors never quote input values).
func classifyDecodeError(service string, err error) error {
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		return scrubTypeError(ute)
	}
	if field, ok := unknownFieldName(err); ok {
		return &UnknownFieldError{Service: service, Field: field}
	}
	return err
}

// scrubTypeError reformats a JSON type mismatch without the offending
// value. encoding/json describes values with UnmarshalTypeError.Value
// (for example "number 1e1000"), which can echo secrets back into logs,
// so only the target struct, field, and expected type are kept.
func scrubTypeError(ute *json.UnmarshalTypeError) error {
	if ute.Struct != "" {
		return fmt.Errorf("json: cannot unmarshal into Go struct field %s.%s of type %s", ute.Struct, ute.Field, ute.Type)
	}
	return fmt.Errorf("json: cannot unmarshal into Go value of type %s", ute.Type)
}

// unknownFieldName extracts the field name from a DisallowUnknownFields
// rejection, which encoding/json always renders as
// `json: unknown field "name"`.
func unknownFieldName(err error) (string, bool) {
	field, ok := strings.CutPrefix(err.Error(), `json: unknown field "`)
	if !ok {
		return "", false
	}
	return strings.Trim(field, `"`), true
}
