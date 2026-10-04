package analytics

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestEdgeValidatePropertiesSize_exactLimit(t *testing.T) {
	t.Parallel()

	m := map[string]any{"k": "v"}

	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal err = %v", err)
	}

	if sizeErr := ValidatePropertiesSize(m, len(b)); sizeErr != nil {
		t.Fatalf("size == limit err = %v, want nil", sizeErr)
	}

	overErr := ValidatePropertiesSize(m, len(b)-1)

	var sle *SizeLimitError
	if !errors.As(overErr, &sle) {
		t.Fatalf("size > limit err = %v, want *SizeLimitError", overErr)
	}
}

func TestEdgeValidateBounds_nilMap(t *testing.T) {
	t.Parallel()

	if err := ValidateBounds(nil, 1, 16); err != nil {
		t.Fatalf("ValidateBounds(nil) err = %v, want nil", err)
	}
}

func TestEdgeMarshalValidated_emptyMap(t *testing.T) {
	t.Parallel()

	b, err := MarshalValidated(map[string]any{}, 16)
	if err != nil {
		t.Fatalf("MarshalValidated(empty) err = %v, want nil", err)
	}

	if string(b) != "{}" {
		t.Fatalf("MarshalValidated(empty) = %q, want {}", b)
	}
}
