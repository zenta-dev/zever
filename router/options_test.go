package router

import (
	"errors"
	"strings"
	"testing"
)

func TestOptions_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		opts    Options
		wantErr bool
		check   func(t *testing.T, err error)
	}{
		{
			name:    "zero passes defaults",
			opts:    Options{},
			wantErr: false,
		},
		{
			name:    "empty app name passes",
			opts:    Options{AppName: ""},
			wantErr: false,
		},
		{
			name:    "typical name passes",
			opts:    Options{AppName: "shop"},
			wantErr: false,
		},
		{
			name:    "boundary 64 passes",
			opts:    Options{AppName: strings.Repeat("a", 64)},
			wantErr: false,
		},
		{
			name:    "boundary 65 fails",
			opts:    Options{AppName: strings.Repeat("a", 65)},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, ErrInvalidOptions) {
					t.Fatalf("err = %v, want ErrInvalidOptions", err)
				}
				if !strings.Contains(err.Error(), "at most 64") {
					t.Fatalf("err = %q, want length reason", err.Error())
				}
			},
		},
		{
			name:    "long 100 fails",
			opts:    Options{AppName: strings.Repeat("z", 100)},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "at most 64") {
					t.Fatalf("err = %q", err.Error())
				}
			},
		},
		{
			name:    "newline control fails",
			opts:    Options{AppName: "ab\ncd"},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "control") {
					t.Fatalf("err = %q, want control reason", err.Error())
				}
				if !errors.Is(err, ErrInvalidOptions) {
					t.Fatal("want ErrInvalidOptions")
				}
			},
		},
		{
			name:    "tab control fails",
			opts:    Options{AppName: "ab\tcd"},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "control") {
					t.Fatalf("err = %q", err.Error())
				}
			},
		},
		{
			name:    "nul control fails",
			opts:    Options{AppName: "ab\x00cd"},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "control") {
					t.Fatalf("err = %q", err.Error())
				}
			},
		},
		{
			name:    "unicode printable passes",
			opts:    Options{AppName: "café-日本語-app_01"},
			wantErr: false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.opts.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected err = %v", err)
			}
			if tc.wantErr && tc.check != nil {
				tc.check(t, err)
			}
		})
	}
}

func TestOptions_Validate_multiple_errors_Join(t *testing.T) {
	t.Parallel()
	opts := Options{AppName: strings.Repeat("x", 65) + "\n"}
	err := opts.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "at most 64") {
		t.Fatalf("err = %q, want length reason", err.Error())
	}
	if !strings.Contains(err.Error(), "control") {
		t.Fatalf("err = %q, want control reason", err.Error())
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err type = %T, want *InvalidOptionsError", err)
	}
}
