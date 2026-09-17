package secrets

import (
	"strings"
	"testing"
)

func TestParseOptions_nil_returnsZero(t *testing.T) {
	t.Parallel()

	o, err := ParseOptions(nil)
	if err != nil {
		t.Fatalf("ParseOptions(nil) error = %v", err)
	}

	if o != (Options{}) {
		t.Errorf("ParseOptions(nil) = %+v, want zero", o)
	}
}

func TestParseOptions_valid(t *testing.T) {
	t.Parallel()

	o, err := ParseOptions(map[string]any{
		"addr":       "https://vault.example:8200",
		"token":      "s.abc",
		"mount":      "secret",
		"project_id": "proj-1",
		"prefix":     "app_",
		"region":     "us-east-1",
	})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}

	if o.Addr != "https://vault.example:8200" || o.Token != "s.abc" || o.Mount != "secret" ||
		o.ProjectID != "proj-1" || o.Prefix != "app_" || o.Region != "us-east-1" {
		t.Errorf("ParseOptions() = %+v, fields not populated", o)
	}
}

func TestParseOptions_unknownKey_returnsError(t *testing.T) {
	t.Parallel()

	_, err := ParseOptions(map[string]any{"nope": "x"})
	if err == nil {
		t.Fatal("ParseOptions(unknown) = nil, want error")
	}

	if !strings.Contains(err.Error(), "unknown option") {
		t.Errorf("error = %q, want unknown option", err.Error())
	}
}

func TestParseOptions_wrongType_returnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key   string
		value any
	}{
		{key: "addr", value: 42},
		{key: "token", value: true},
		{key: "mount", value: []string{"x"}},
		{key: "project_id", value: 1.5},
		{key: "prefix", value: map[string]any{}},
		{key: "region", value: int64(7)},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()

			_, err := ParseOptions(map[string]any{tt.key: tt.value})
			if err == nil {
				t.Fatalf("ParseOptions(%q=%T) = nil, want error", tt.key, tt.value)
			}

			if !strings.Contains(err.Error(), "must be a string") {
				t.Errorf("error = %q, want must be a string", err.Error())
			}
		})
	}
}

func TestOptionsValidate_zero_returnsNil(t *testing.T) {
	t.Parallel()

	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
