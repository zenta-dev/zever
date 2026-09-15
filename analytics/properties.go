package analytics

import (
	"encoding/json"
)

// ValidatePropertiesSize checks that the JSON-encoded size of m does not exceed limit.
func ValidatePropertiesSize(m map[string]any, limit int) error {
	if m == nil {
		return nil
	}

	if limit <= 0 {
		limit = DefaultMaxPropertiesBytes
	}

	b, err := json.Marshal(m)
	if err != nil {
		return err
	}

	if len(b) > limit {
		return &SizeLimitError{Size: len(b), Limit: limit}
	}

	return nil
}

// MarshalValidated marshals m and validates the size in a single pass, returning the JSON bytes.
func MarshalValidated(m map[string]any, limit int) ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}

	if limit <= 0 {
		limit = DefaultMaxPropertiesBytes
	}

	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	if len(b) > limit {
		return nil, &SizeLimitError{Size: len(b), Limit: limit}
	}

	return b, nil
}

// ValidateBounds checks the property count then the JSON-encoded size of m.
func ValidateBounds(m map[string]any, maxProperties int, maxBytes int) error {
	if maxProperties > 0 && len(m) > maxProperties {
		return &CountLimitError{Count: len(m), Limit: maxProperties}
	}

	return ValidatePropertiesSize(m, maxBytes)
}
