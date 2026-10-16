package db

import (
	"strings"
	"testing"
)

// TestKeyNeverContainsSessionID proves the kv key is derived from a hash, so
// a table dump never exposes a usable bearer session ID.
func TestKeyNeverContainsSessionID(t *testing.T) {
	t.Parallel()

	const id = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	d := &driver{prefix: "sess"}
	got := d.key(id)

	if strings.Contains(got, id) {
		t.Fatalf("key %q contains the raw session ID", got)
	}

	if want := "sess:" + hashID(id); got != want || len(got) != len("sess:")+64 {
		t.Errorf("key = %q, want %q (64 hex chars after the prefix)", got, want)
	}

	if d.key(id) != got || d.key(id[:63]+"0") == got {
		t.Error("key must be deterministic and ID-sensitive")
	}
}
