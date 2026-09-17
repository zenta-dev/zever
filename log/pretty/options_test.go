package pretty

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/log"
)

func TestParseOptions_defaults(t *testing.T) {
	t.Parallel()

	o, err := ParseOptions(nil)
	if err != nil {
		t.Fatalf("ParseOptions(nil): %v", err)
	}

	if o.Level != "info" {
		t.Fatalf("Level = %q, want info", o.Level)
	}

	if o.Color != nil {
		t.Fatalf("Color = %v, want nil", o.Color)
	}

	if err := o.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
}

func TestParseOptions_custom(t *testing.T) {
	t.Parallel()

	o, err := ParseOptions(map[string]any{"level": "debug", "color": true})
	if err != nil {
		t.Fatalf("ParseOptions: %v", err)
	}

	if o.Level != "debug" {
		t.Fatalf("Level = %q, want debug", o.Level)
	}

	if o.Color == nil || !*o.Color {
		t.Fatal("Color should be true")
	}

	if err := o.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
}

func TestParseOptions_rejectsUnknownOption(t *testing.T) {
	t.Parallel()

	_, err := ParseOptions(map[string]any{"bogus": 1})
	if err == nil {
		t.Fatal("ParseOptions(bogus): want error, got nil")
	}

	var unknown *UnknownOptionError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %T, want *UnknownOptionError", err)
	}

	if !errors.Is(err, ErrUnknownOption) {
		t.Fatalf("error = %v, want ErrUnknownOption", err)
	}
}

func TestParseOptions_rejectsWrongTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
	}{
		{"levelInt", map[string]any{"level": 1}},
		{"colorString", map[string]any{"color": "yes"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParseOptions(tt.m); err == nil {
				t.Fatal("ParseOptions: want error, got nil")
			}
		})
	}
}

func TestOptions_Validate_rejectsBadLevel(t *testing.T) {
	t.Parallel()

	o := Options{Level: "verbose"}

	err := o.Validate()
	if err == nil {
		t.Fatal("Validate(bad level): want error, got nil")
	}

	if !errors.Is(err, log.ErrInvalidLevel) {
		t.Fatalf("error = %v, want log.ErrInvalidLevel", err)
	}
}

func TestOptions_Validate_emptyLevelOK(t *testing.T) {
	t.Parallel()

	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate(empty): %v", err)
	}
}

func TestUnknownOptionError_message(t *testing.T) {
	t.Parallel()

	err := &UnknownOptionError{Option: "bogus"}
	if got := err.Error(); got == "" {
		t.Fatal("Error() is empty")
	}

	if !errors.Is(err, ErrUnknownOption) {
		t.Fatalf("Unwrap: %v does not match ErrUnknownOption", err)
	}
}

func TestInvalidOptionError_message(t *testing.T) {
	t.Parallel()

	err := &InvalidOptionError{Option: "color"}

	if got := err.Error(); got == "" {
		t.Fatal("Error() is empty")
	}

	if !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("Unwrap: %v does not match ErrInvalidOption", err)
	}
}

func TestNew_nameAndEnabled(t *testing.T) {
	t.Parallel()

	l := New(Options{Level: "debug"})
	if l.Name() != "pretty" {
		t.Fatalf("Name() = %q, want pretty", l.Name())
	}

	if !l.Enabled(log.LevelDebug) {
		t.Fatal("Enabled(debug) = false, want true")
	}

	if err := l.Sync(); err != nil {
		t.Fatalf("Sync(): %v", err)
	}
}

func TestNew_badLevelFallsBackToInfo(t *testing.T) {
	t.Parallel()

	l := New(Options{Level: "verbose"})
	if l.Enabled(log.LevelDebug) {
		t.Fatal("Enabled(debug) with bad level = true, want false (info fallback)")
	}

	if !l.Enabled(log.LevelInfo) {
		t.Fatal("Enabled(info) with bad level = false, want true")
	}
}
