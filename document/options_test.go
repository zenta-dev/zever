package document

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOptions_Validate_zero_valid(t *testing.T) {
	t.Parallel()
	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate zero err = %v, want nil", err)
	}
}

func TestOptions_Validate_violations_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		opts    Options
		reasons []string
	}{
		{"negative timeout", Options{Timeout: -time.Second}, []string{"timeout"}},
		{"negative quality", Options{Quality: -1}, []string{"quality"}},
		{"quality too high", Options{Quality: 101}, []string{"quality"}},
		{"negative dpi", Options{DPI: -1}, []string{"dpi"}},
		{"negative output bytes", Options{MaxOutputBytes: -1}, []string{"max output bytes"}},
		{"negative runs", Options{LatexRuns: -1}, []string{"latex runs"}},
		{"no scheme", Options{Endpoint: "example.com/render"}, []string{"scheme"}},
		{"no host", Options{Endpoint: "https:///path"}, []string{"host"}},
		{"garbage", Options{Endpoint: "http://[::1"}, []string{"valid URL"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.opts.Validate()
			if !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Validate err = %v, want ErrInvalidOptions", err)
			}

			for _, r := range tc.reasons {
				if !strings.Contains(err.Error(), r) {
					t.Errorf("Validate err %q missing %q", err.Error(), r)
				}
			}

			var ioe *InvalidOptionsError
			if !errors.As(err, &ioe) {
				t.Errorf("err %T is not *InvalidOptionsError", err)
			}
		})
	}
}

func TestOptions_Validate_multiple_joined(t *testing.T) {
	t.Parallel()

	err := Options{Timeout: -time.Second, DPI: -1, Endpoint: "example.com/render"}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate err = %v, want ErrInvalidOptions", err)
	}

	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("err %q missing timeout reason", err.Error())
	}

	if !strings.Contains(err.Error(), "dpi") {
		t.Errorf("err %q missing dpi reason", err.Error())
	}

	if !strings.Contains(err.Error(), "scheme") {
		t.Errorf("err %q missing scheme reason", err.Error())
	}
}

func TestOptions_Validate_endpoints_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"https", "https://example.com/render", false},
		{"http localhost", "http://localhost:8080", false},
		{"empty", "", false},
		{"no scheme", "example.com/render", true},
		{"no host", "https:///path", true},
		{"garbage", "http://[::1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := Options{Endpoint: tc.endpoint}.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate(%q) = nil, want error", tc.endpoint)
			}

			if !tc.wantErr && err != nil {
				t.Errorf("Validate(%q) = %v, want nil", tc.endpoint, err)
			}
		})
	}
}

func TestClampQuality_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   int
		want int
	}{
		{"below", -5, 0},
		{"zero", 0, 0},
		{"mid", 50, 50},
		{"max", 100, 100},
		{"above", 150, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClampQuality(tc.in); got != tc.want {
				t.Errorf("ClampQuality(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestOptions_defaults_values(t *testing.T) {
	t.Parallel()

	if DefaultTimeout != 30*time.Second {
		t.Errorf("DefaultTimeout = %v, want %v", DefaultTimeout, 30*time.Second)
	}

	if DefaultMaxSourceBytes != 10<<20 {
		t.Errorf("DefaultMaxSourceBytes = %d, want %d", DefaultMaxSourceBytes, 10<<20)
	}

	if DefaultMaxOutputBytes != 64<<20 {
		t.Errorf("DefaultMaxOutputBytes = %d, want %d", DefaultMaxOutputBytes, 64<<20)
	}

	if DefaultLatexDPI != 150 {
		t.Errorf("DefaultLatexDPI = %d, want 150", DefaultLatexDPI)
	}

	if DefaultLatexQuality != 80 {
		t.Errorf("DefaultLatexQuality = %d, want 80", DefaultLatexQuality)
	}

	if DefaultLatexCommand != "pdflatex" {
		t.Errorf("DefaultLatexCommand = %q, want %q", DefaultLatexCommand, "pdflatex")
	}

	if DefaultLatexConverter != "pdftoppm" {
		t.Errorf("DefaultLatexConverter = %q, want %q", DefaultLatexConverter, "pdftoppm")
	}
}
