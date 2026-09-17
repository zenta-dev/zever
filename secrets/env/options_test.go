package env

import (
	"strings"
	"testing"
)

func TestParseOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    map[string]any
		want    Options
		wantErr bool
	}{
		{name: "empty map", opts: map[string]any{}, wantErr: true},
		{name: "nil map", opts: nil, wantErr: true},
		{name: "empty prefix", opts: map[string]any{"prefix": ""}, wantErr: true},
		{name: "missing prefix", opts: map[string]any{"other": "x"}, wantErr: true},
		{name: "non-string prefix", opts: map[string]any{"prefix": 42}, wantErr: true},
		{name: "slash prefix", opts: map[string]any{"prefix": "a/b"}, wantErr: true},
		{name: "dotdot prefix", opts: map[string]any{"prefix": "a..b"}, wantErr: true},
		{name: "space prefix", opts: map[string]any{"prefix": "a b"}, wantErr: true},
		{
			name: "valid prefix",
			opts: map[string]any{"prefix": "MYAPP_"},
			want: Options{Prefix: "MYAPP_"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseOptions(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}

			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOptionsValidate_joinsViolations(t *testing.T) {
	t.Parallel()

	err := Options{Prefix: "a/b c"}.Validate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	for _, want := range []string{"slash or dot-dot", "spaces"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}

	if err := (Options{Prefix: "OK_"}).Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
