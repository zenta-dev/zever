package analytics

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidatePropertiesSize_nil_returnsNil(t *testing.T) {
	t.Parallel()
	if err := ValidatePropertiesSize(nil, 10); err != nil {
		t.Fatalf("nil err = %v, want nil", err)
	}
}

func TestValidatePropertiesSize_within_returnsNil(t *testing.T) {
	t.Parallel()
	if err := ValidatePropertiesSize(map[string]any{"a": "b"}, 1024); err != nil {
		t.Fatalf("within err = %v, want nil", err)
	}
}

func TestValidatePropertiesSize_over_returnsSizeLimit(t *testing.T) {
	t.Parallel()
	m := map[string]any{"k": strings.Repeat("x", 100)}
	err := ValidatePropertiesSize(m, 10)
	var sle *SizeLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("err %T is not *SizeLimitError", err)
	}
	if !errors.Is(err, ErrPropertiesTooLarge) {
		t.Fatalf("err = %v, want ErrPropertiesTooLarge", err)
	}
	if sle.Limit != 10 {
		t.Errorf("Limit = %d, want 10", sle.Limit)
	}
}

func TestValidatePropertiesSize_nonPositiveLimit_usesDefault(t *testing.T) {
	t.Parallel()
	if err := ValidatePropertiesSize(map[string]any{"a": "b"}, 0); err != nil {
		t.Fatalf("limit 0 err = %v, want nil", err)
	}
	if err := ValidatePropertiesSize(map[string]any{"a": "b"}, -1); err != nil {
		t.Fatalf("limit -1 err = %v, want nil", err)
	}
}

func TestValidatePropertiesSize_cyclic_returnsUnsupportedValue(t *testing.T) {
	t.Parallel()
	m := map[string]any{}
	m["self"] = m
	err := ValidatePropertiesSize(m, 1024)
	var uve *json.UnsupportedValueError
	if !errors.As(err, &uve) {
		t.Fatalf("err %T is not *json.UnsupportedValueError", err)
	}
}

func TestMarshalValidated_null_returnsNull(t *testing.T) {
	t.Parallel()
	b, err := MarshalValidated(nil, 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if string(b) != "null" {
		t.Errorf("got %q, want %q", b, "null")
	}
}

func TestMarshalValidated_ok_returnsBytes(t *testing.T) {
	t.Parallel()
	b, err := MarshalValidated(map[string]any{"a": "b"}, 1024)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal err = %v", err)
	}
	if decoded["a"] != "b" {
		t.Errorf("decoded = %v, want a=b", decoded)
	}
}

func TestMarshalValidated_over_returnsSizeLimit(t *testing.T) {
	t.Parallel()
	m := map[string]any{"k": strings.Repeat("x", 100)}
	_, err := MarshalValidated(m, 10)
	var sle *SizeLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("err %T is not *SizeLimitError", err)
	}
	if !errors.Is(err, ErrPropertiesTooLarge) {
		t.Fatalf("err = %v, want ErrPropertiesTooLarge", err)
	}
}

func TestMarshalValidated_defaultLimit_returnsBytes(t *testing.T) {
	t.Parallel()
	b, err := MarshalValidated(map[string]any{"a": "b"}, 0)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(b) == 0 {
		t.Fatal("empty bytes")
	}
}

func TestMarshalValidated_cyclic_returnsUnsupportedValue(t *testing.T) {
	t.Parallel()
	m := map[string]any{}
	m["self"] = m
	_, err := MarshalValidated(m, 1024)
	var uve *json.UnsupportedValueError
	if !errors.As(err, &uve) {
		t.Fatalf("err %T is not *json.UnsupportedValueError", err)
	}
}

func TestValidateBounds_countFirst_ordering(t *testing.T) {
	t.Parallel()
	m := map[string]any{"a": strings.Repeat("x", 100), "b": "y"}
	err := ValidateBounds(m, 1, 1)
	var cle *CountLimitError
	if !errors.As(err, &cle) {
		t.Fatalf("err %T is not *CountLimitError", err)
	}
	if !errors.Is(err, ErrTooManyProperties) {
		t.Fatalf("err = %v, want ErrTooManyProperties", err)
	}
	if cle.Count != 2 || cle.Limit != 1 {
		t.Errorf("got count=%d limit=%d, want 2/1", cle.Count, cle.Limit)
	}
}

func TestValidateBounds_table(t *testing.T) {
	t.Parallel()
	big := map[string]any{"k": strings.Repeat("x", 100)}
	cases := []struct {
		name  string
		m     map[string]any
		maxN  int
		maxB  int
		want  error
		wantN bool
	}{
		{"nil ok", nil, 1, 10, nil, false},
		{"within", map[string]any{"a": "b"}, 5, 1024, nil, false},
		{"count breach", map[string]any{"a": 1, "b": 2}, 1, 1024, ErrTooManyProperties, false},
		{"size breach", big, 10, 5, ErrPropertiesTooLarge, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateBounds(tc.m, tc.maxN, tc.maxB)
			if tc.want == nil && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
