package redis

import (
	"strings"
	"testing"
)

// TestKeyNeverContainsSessionID proves the Redis key is derived from a hash,
// so a key listing never exposes a usable bearer session ID.
func TestKeyNeverContainsSessionID(t *testing.T) {
	t.Parallel()

	const id = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	st := &store{prefix: "sess"}
	got := st.key(id)

	if strings.Contains(got, id) {
		t.Fatalf("key %q contains the raw session ID", got)
	}

	// sha256 of the ID above, lowercase hex.
	if want := "sess:" + hashID(id); got != want || len(got) != len("sess:")+64 {
		t.Errorf("key = %q, want %q (64 hex chars after the prefix)", got, want)
	}

	if st.key(id) != got || st.key(id[:63]+"0") == got {
		t.Error("key must be deterministic and ID-sensitive")
	}
}
