package sqlite

import "testing"

// TestParseVersion_valid_inputs verifies accepted version shapes.
func TestParseVersion_valid_inputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want version
	}{
		{name: "full", in: "3.46.0", want: version{major: 3, minor: 46, patch: 0}},
		{name: "older", in: "3.30.0", want: version{major: 3, minor: 30, patch: 0}},
		{name: "patch", in: "3.29.5", want: version{major: 3, minor: 29, patch: 5}},
		{name: "missing patch", in: "3.30", want: version{major: 3, minor: 30, patch: 0}},
		{name: "major only", in: "3", want: version{major: 3, minor: 0, patch: 0}},
		{name: "suffix", in: "3.46.0-community", want: version{major: 3, minor: 46, patch: 0}},
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
		{name: "dash only", in: "-log"},
		{name: "non-numeric minor", in: "3.x.0"},
		{name: "non-numeric", in: "three.forty"},
		{name: "too many components", in: "3.46.0.1"},
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
		{name: "equal", v: version{major: 3, minor: 46, patch: 0}, major: 3, minor: 46, patch: 0, want: true},
		{name: "above patch", v: version{major: 3, minor: 46, patch: 0}, major: 3, minor: 46, patch: 1, want: false},
		{name: "below minor", v: version{major: 3, minor: 29, patch: 5}, major: 3, minor: 30, patch: 0, want: false},
		{name: "above major", v: version{major: 4, minor: 0, patch: 0}, major: 3, minor: 46, patch: 0, want: true},
		{name: "below major", v: version{major: 2, minor: 9, patch: 9}, major: 3, minor: 0, patch: 0, want: false},
		{name: "above minor", v: version{major: 3, minor: 46, patch: 0}, major: 3, minor: 30, patch: 0, want: true},
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
		{name: "current", in: "3.46.0", want: version{major: 3, minor: 46, patch: 0}},
		{name: "floor", in: "3.30.0", want: version{major: 3, minor: 30, patch: 0}},
		{name: "suffix", in: "3.46.0-community", want: version{major: 3, minor: 46, patch: 0}},
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
		{name: "non-numeric", in: "3.x.0"},
		{name: "too many", in: "3.46.0.1"},
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
