package config

import (
	"strings"
	"testing"
)

// FuzzSetOptionField proves the env overlay's reflection path never panics
// on arbitrary field names and values: every input yields a clean set or a
// clean typed error (UnknownFieldError / InvalidOptionsError), and error
// text never echoes the value (secrets must not leak into diagnostics).
// Values carry a fixed canary prefix so a leak finding cannot false-positive
// on template vocabulary (service names, "unknown field", expected kinds).
func FuzzSetOptionField(f *testing.F) {
	for _, seed := range []struct{ field, value string }{
		{"maxconns", "20"},
		{"maxconns", "not-a-number"},
		{"dsn", "postgres://u@h/d"},
		{"nosuchfield", "x"},
		{"", ""},
		{"adapter", "sqlite"},
		{"dedicatedpool", "true"},
		{"sweepinterval", "5s"},
		{"sweepinterval", "bogus"},
		{"maxconns", "-1"},
		{"\x00\xff", "\x00\xff"},
		{strings.Repeat("a_", 50), strings.Repeat("b", 1000)},
	} {
		f.Add(seed.field, seed.value)
	}

	const canary = "s3cr3t-v4lu3-"

	f.Fuzz(func(t *testing.T, field, value string) {
		t.Parallel()

		// A field naming the canary would echo it legitimately via the
		// field slot; that input tests nothing about values.
		if strings.Contains(strings.ToLower(field), canary) {
			return
		}

		cfg := Default()
		_, opts, ok := serviceRefs(cfg, "db")
		if !ok {
			t.Fatal("serviceRefs(db) missing")
		}

		err := setOptionField("db", opts, []string{strings.ToLower(field)}, canary+value, "DB_TEST")
		if err == nil {
			return
		}

		if strings.Contains(err.Error(), canary) {
			t.Fatalf("setOptionField(%q) error echoes the value: %v", field, err)
		}
	})
}

// FuzzMatchService proves service-head matching never panics on arbitrary
// input, including non-ASCII heads that fold oddly under ToLower.
func FuzzMatchService(f *testing.F) {
	for _, seed := range []string{"", "DB", "db", "AUTH", "K", "KELVIN", "\u212a", "\x00\xff", "a_b"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, head string) {
		t.Parallel()

		_, _ = matchService(head)
	})
}

// FuzzCamelToSnake proves the CamelCase -> snake_case helper never panics on
// arbitrary input (including non-ASCII and control bytes): it is pure string
// rewriting with no integer conversions, so G115 cannot fire and the prev/next
// indexing only runs on the ASCII-uppercase branch.
func FuzzCamelToSnake(f *testing.F) {
	for _, seed := range []string{
		"", "A", "APIKey", "dbMaxConns", "HTTPServer", "OIDCIssuerURL",
		"DB2Conn", "Ünicode", "caféAPIKey", "\x00\xff", strings.Repeat("A", 100),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, s string) {
		t.Parallel()

		_ = camelToSnake(s)
	})
}
