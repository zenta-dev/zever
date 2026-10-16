package middleware

import (
	"errors"
	"strings"
	"testing"
)

// TestRedactPanicValue pins that secrets embedded in a panic value (DSN
// credentials, bearer tokens, key=value secrets, opaque session-like IDs) never
// reach the logs, while ordinary messages survive.
func TestRedactPanicValue(t *testing.T) {
	t.Parallel()

	const sid = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name  string
		in    any
		leak  string
		keeps string
	}{
		{"dsn userinfo", errors.New("dial postgres://app:hunter2@db:5432/app: refused"), "hunter2", "dial"},
		{"bearer", "bad header Authorization: Bearer abc.def-123", "abc.def-123", "bad header"},
		{"kv password", "login failed password=s3cr3t user=bob", "s3cr3t", "user=bob"},
		{"json token", `{"token":"tok_live_123","ok":true}`, "tok_live_123", `"ok":true`},
		{"api key", "api_key: sk-test-9999 rejected", "sk-test-9999", "rejected"},
		{"session id", "session " + sid + " corrupt", sid, "corrupt"},
		{"plain", "index out of range [3] with length 2", "", "index out of range [3] with length 2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := redactPanicValue(tc.in)
			if tc.leak != "" && strings.Contains(got, tc.leak) {
				t.Errorf("redacted value still contains %q: %q", tc.leak, got)
			}

			if !strings.Contains(got, tc.keeps) {
				t.Errorf("redacted value lost %q: %q", tc.keeps, got)
			}
		})
	}
}

// TestRedactPanicValueTruncates pins the length cap so a panic carrying a huge
// payload cannot flood or smuggle data into the logs.
func TestRedactPanicValueTruncates(t *testing.T) {
	t.Parallel()

	got := redactPanicValue(strings.Repeat("x ", 5000))
	if len(got) > maxPanicValueLen+len("...(truncated)") {
		t.Errorf("len = %d, want <= %d", len(got), maxPanicValueLen+len("...(truncated)"))
	}

	if !strings.HasSuffix(got, "...(truncated)") {
		t.Errorf("missing truncation marker: %q", got[len(got)-20:])
	}
}
