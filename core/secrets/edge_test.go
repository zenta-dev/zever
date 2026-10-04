package secrets_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

func TestValidateName_boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"single char", "a", false},
		{"very long", strings.Repeat("a", 1<<16), false},
		{"leading dot", ".hidden", false},
		{"trailing dot", "name.", false},
		{"single dot", ".", false},
		{"double dot", "..", true},
		{"triple dot", "...", true},
		{"unicode", "café", true},
		{"newline", "a\nb", true},
		{"nul", "a\x00b", true},
		{"backslash", "a\\b", true},
		{"colon", "a:b", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := secrets.ValidateName(tc.input)
			if tc.wantErr && !errors.Is(err, secrets.ErrInvalidKey) {
				t.Fatalf("ValidateName(%q) err = %v, want ErrInvalidKey", tc.input, err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateName(%q) err = %v, want nil", tc.input, err)
			}
		})
	}
}

func TestValidateName_neverEchoesFullInput(t *testing.T) {
	t.Parallel()

	err := secrets.ValidateName("key;DROP")
	if !errors.Is(err, secrets.ErrInvalidKey) {
		t.Fatalf("err = %v, want ErrInvalidKey", err)
	}
}

func TestOptionsValidate_alwaysNil(t *testing.T) {
	t.Parallel()

	cases := []secrets.Options{
		{},
		{Addr: "", Token: ""},
		{Addr: "not-a-url", Prefix: "../escape"},
		{ProjectID: strings.Repeat("x", 1<<16)},
	}

	for i, opts := range cases {
		if err := opts.Validate(); err != nil {
			t.Fatalf("case %d: Validate() err = %v, want nil", i, err)
		}
	}
}

func TestParseAdapter_custom(t *testing.T) {
	t.Parallel()

	a, err := secrets.ParseAdapter("custom")
	if err != nil {
		t.Fatalf("ParseAdapter(custom) error = %v", err)
	}
	if a != secrets.Adapter("custom") {
		t.Fatalf("ParseAdapter(custom) = %v, want custom", a)
	}
	if a.String() != "custom" {
		t.Fatalf("String() = %q, want custom", a.String())
	}
}
