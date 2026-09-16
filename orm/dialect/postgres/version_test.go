package postgres

import "testing"

// TestParseVersion_valid_inputs verifies accepted version shapes.
func TestParseVersion_valid_inputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want version
	}{
		{name: "major minor", in: "16.0", want: version{major: 16, minor: 0, patch: 0}},
		{name: "patch", in: "14.2", want: version{major: 14, minor: 2, patch: 0}},
		{name: "old", in: "11.5", want: version{major: 11, minor: 5, patch: 0}},
		{name: "major only", in: "12", want: version{major: 12, minor: 0, patch: 0}},
		{name: "suffix", in: "14.0-community", want: version{major: 14, minor: 0, patch: 0}},
		{name: "distro suffix", in: "16.3 (Ubuntu 16.3-1)", want: version{major: 16, minor: 3, patch: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseVersion(tt.in)
			if err != nil {
				t.Fatalf("parseVersion(%q) error: %v", tt.in, err)
			}

			if got != tt.want {
				t.Fatalf("parseVersion(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

// TestParseVersion_invalid_inputs verifies rejected version shapes.
func TestParseVersion_invalid_inputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "non-numeric", in: "abc"},
		{name: "too many components", in: "1.2.3.4"},
		{name: "non-numeric minor", in: "16.x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := parseVersion(tt.in); err == nil {
				t.Fatalf("parseVersion(%q) = nil error, want error", tt.in)
			}
		})
	}
}

// TestVersion_atLeast verifies version comparison boundaries.
func TestVersion_atLeast(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		v     version
		major int
		minor int
		patch int
		want  bool
	}{
		{name: "equal", v: version{major: 16, minor: 0, patch: 0}, major: 16, minor: 0, patch: 0, want: true},
		{name: "below minor", v: version{major: 14, minor: 13, patch: 0}, major: 15, minor: 0, patch: 0, want: false},
		{name: "above minor", v: version{major: 16, minor: 2, patch: 0}, major: 15, minor: 0, patch: 0, want: true},
		{name: "below major", v: version{major: 11, minor: 5, patch: 0}, major: 12, minor: 0, patch: 0, want: false},
		{name: "above major", v: version{major: 17, minor: 0, patch: 0}, major: 16, minor: 0, patch: 0, want: true},
		{name: "below patch", v: version{major: 15, minor: 0, patch: 0}, major: 15, minor: 0, patch: 1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.v.atLeast(tt.major, tt.minor, tt.patch); got != tt.want {
				t.Fatalf("%+v.atLeast(%d, %d, %d) = %v, want %v",
					tt.v, tt.major, tt.minor, tt.patch, got, tt.want)
			}
		})
	}
}

// TestNewWithVersion_valid verifies version pinning.
func TestNewWithVersion_valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want version
	}{
		{name: "current", in: "16.0", want: version{major: 16, minor: 0, patch: 0}},
		{name: "old", in: "11.5", want: version{major: 11, minor: 5, patch: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewWithVersion(tt.in)
			if err != nil {
				t.Fatalf("NewWithVersion(%q) error: %v", tt.in, err)
			}

			if d.version != tt.want {
				t.Fatalf("NewWithVersion(%q).version = %+v, want %+v", tt.in, d.version, tt.want)
			}
		})
	}
}

// TestNewWithVersion_invalid verifies bad versions are errors, never
// silent fallbacks.
func TestNewWithVersion_invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "non-numeric", in: "abc"},
		{name: "too many", in: "1.2.3.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewWithVersion(tt.in); err == nil {
				t.Fatalf("NewWithVersion(%q) = nil error, want error", tt.in)
			}
		})
	}
}
