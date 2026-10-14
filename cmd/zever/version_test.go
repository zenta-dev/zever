package main

import "testing"

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, ldflags, info, fallback, want string
	}{
		{"ldflags wins", "1.2.3", "v9.9.9", "0.7.0", "1.2.3"},
		{"ldflags v prefix stripped", "v1.2.3", "", "0.7.0", "1.2.3"},
		{"build info used", "", "v0.8.1", "0.7.0", "0.8.1"},
		{"devel falls back", "", "(devel)", "0.7.0\n", "0.7.0"},
		{"empty info falls back", "", "", "0.7.0", "0.7.0"},
		{"pseudo-version falls back", "", "v0.7.1-0.20260101010101-abcdef123456", "0.7.0", "0.7.0"},
		{"dirty falls back", "", "v0.8.1+dirty", "0.7.0", "0.7.0"},
		{"prerelease accepted", "", "v0.8.0-rc.1", "0.7.0", "0.8.0-rc.1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveVersion(tc.ldflags, tc.info, tc.fallback); got != tc.want {
				t.Fatalf("resolveVersion(%q,%q,%q) = %q, want %q", tc.ldflags, tc.info, tc.fallback, got, tc.want)
			}
		})
	}
}

// TestFallbackVersionIsSemver guards the embedded VERSION file shape.
func TestFallbackVersionIsSemver(t *testing.T) {
	t.Parallel()

	if v := resolveVersion("", "", fallbackVersion); !semverRe.MatchString(v) {
		t.Fatalf("VERSION %q is not X.Y.Z", fallbackVersion)
	}
}
