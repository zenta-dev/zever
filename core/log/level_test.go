package log

import (
	"errors"
	"testing"
)

func TestLevel_String_returnsName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Level
		want string
	}{
		{name: "debug", in: LevelDebug, want: "debug"},
		{name: "info", in: LevelInfo, want: "info"},
		{name: "warn", in: LevelWarn, want: "warn"},
		{name: "error", in: LevelError, want: "error"},
		{name: "fatal", in: LevelFatal, want: "fatal"},
		{name: "unknown", in: Level(99), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.in.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseLevel_valid_returnsLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want Level
	}{
		{name: "debug", in: "debug", want: LevelDebug},
		{name: "info", in: "info", want: LevelInfo},
		{name: "warn", in: "warn", want: LevelWarn},
		{name: "error", in: "error", want: LevelError},
		{name: "fatal", in: "fatal", want: LevelFatal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseLevel(tt.in)
			if err != nil {
				t.Fatalf("ParseLevel() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("ParseLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseLevel_invalid_returnsInvalidLevelError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "bogus", in: "bogus"},
		{name: "uppercase rejected", in: "DEBUG"},
		{name: "whitespace rejected", in: " info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseLevel(tt.in)
			if err == nil {
				t.Fatal("ParseLevel() error = nil, want ErrInvalidLevel")
			}

			if got != LevelDebug {
				t.Errorf("ParseLevel() level = %v, want LevelDebug zero fallback", got)
			}

			if !errors.Is(err, ErrInvalidLevel) {
				t.Errorf("errors.Is(err, ErrInvalidLevel) = false (err = %v)", err)
			}

			var invErr *InvalidLevelError
			if !errors.As(err, &invErr) {
				t.Fatalf("errors.As(err, InvalidLevelError) = false (err = %T %v)", err, err)
			}

			if invErr.Level != tt.in {
				t.Errorf("InvalidLevelError.Level = %q, want %q", invErr.Level, tt.in)
			}
		})
	}
}
