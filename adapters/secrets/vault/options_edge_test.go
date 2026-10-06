package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEdgeValidate_rejectsMalformedFields covers the Validate branches that
// the New-level tests do not reach: a scheme with no host, mount names with
// spaces or dot-dot segments, and an empty token file.
func TestEdgeValidate_rejectsMalformedFields(t *testing.T) {
	t.Parallel()

	emptyFile := filepath.Join(t.TempDir(), "empty-token")
	if err := os.WriteFile(emptyFile, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}

	cases := []struct {
		name string
		opts Options
		want string
	}{
		{"addr missing host", Options{Addr: "http://", Token: "t", Mount: "secret"}, "addr must include host"},
		{"mount with spaces", Options{Addr: "http://127.0.0.1:8200", Token: "t", Mount: "my mount"}, "mount must not contain"},
		{"mount with dot-dot", Options{Addr: "http://127.0.0.1:8200", Token: "t", Mount: "a..b"}, "mount must not contain"},
		{"token file empty", Options{Addr: "http://127.0.0.1:8200", Mount: "secret", TokenFile: emptyFile}, "token_file must not be empty"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.opts.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.want)
			}
		})
	}
}

// TestEdgeValidate_validOptions covers the all-clear path, including a mount
// that is trimmed to empty being reported as required.
func TestEdgeValidate_validOptions(t *testing.T) {
	t.Parallel()

	if err := (Options{Addr: "http://127.0.0.1:8200", Token: "t", Mount: "secret"}).Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	if err := (Options{Addr: "http://127.0.0.1:8200", Token: "t", Mount: "  "}).Validate(); err == nil {
		t.Fatal("Validate(blank mount) = nil, want error")
	}
}

// TestEdgeResolveToken covers the token-vs-token-file precedence and the
// empty/unreadable file error paths.
func TestEdgeResolveToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	file := filepath.Join(dir, "token")
	if err := os.WriteFile(file, []byte("  from-file\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}

	emptyFile := filepath.Join(dir, "empty")
	if err := os.WriteFile(emptyFile, []byte("\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}

	cases := []struct {
		name    string
		opts    Options
		want    string
		wantErr bool
	}{
		{"inline token wins", Options{Token: " inline "}, "inline", false},
		{"file token trimmed", Options{TokenFile: file}, "from-file", false},
		{"missing file", Options{TokenFile: filepath.Join(dir, "nope")}, "", true},
		{"empty file", Options{TokenFile: emptyFile}, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.opts.resolveToken()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveToken() = %q, nil, want error", got)
				}

				return
			}

			if err != nil || got != tc.want {
				t.Fatalf("resolveToken() = (%q, %v), want (%q, nil)", got, err, tc.want)
			}
		})
	}
}
