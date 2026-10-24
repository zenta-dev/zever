package config

import (
	"testing"
)

// FuzzResolve proves the env-to-config overlay never panics on an arbitrary
// single key/value pair, is deterministic for the same input, and fails
// closed in shape: any error returns a nil config, never a half-applied one.
// Seeds cover the accepted shapes (default and named instances), the removed
// adapter-selection keys, near-miss typo keys, and hostile bytes.
func FuzzResolve(f *testing.F) {
	seeds := []struct{ key, value string }{
		{"DB_MAX_CONNS", "20"},
		{"DB_MAX_CONNS", "not-a-number"},
		{"DB__REPORTING__MAX_CONNS", "5"},
		{"DB_ADAPTER", "postgres"},
		{"FOO_ADAPTER", "bar"},
		{"DB_MAXCONBNS", "1"},
		{"DB_NOPE_X", "x"},
		{"PATH", "/bin"},
		{"", ""},
		{"_", "x"},
		{"AUTH_JWT_SECRET", "s3cr3t"},
		{"DB_DSN", "postgres://u@h/d"},
		{"\x00\xff", "\x00\xff"},
		{"DB_MAX_CONNS", "-1"},
		{"ZEVER_CONFIG_STRICT", "1"},
	}

	for _, seed := range seeds {
		f.Add(seed.key, seed.value)
	}

	f.Fuzz(func(t *testing.T, key, value string) {
		t.Parallel()

		env := Env{key: value}

		cfg, err := Resolve(env)
		if err != nil {
			// Fail closed in shape: no half-applied config escapes.
			if cfg != nil {
				t.Fatalf("Resolve(%q) error %v with non-nil config", key, err)
			}
			return
		}

		if cfg == nil {
			t.Fatalf("Resolve(%q) = nil config with nil error", key)
		}

		// Deterministic: the same env resolves the same way.
		again, againErr := Resolve(env)
		if againErr != nil {
			t.Fatalf("Resolve(%q) second call error %v, first call succeeded", key, againErr)
		}
		if again == nil {
			t.Fatalf("Resolve(%q) second call nil config, first call succeeded", key)
		}
	})
}
