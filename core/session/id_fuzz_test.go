package session

import (
	"encoding/hex"
	"strings"
	"testing"
)

// FuzzValidateID proves ValidateID never panics on arbitrary input and
// accepts exactly the 64-lowercase-hex shape NewID produces. It runs as an
// ordinary seed-corpus regression test under `go test ./core/session/`;
// run with `-fuzz=FuzzValidateID -fuzztime=...` for continuous fuzzing.
func FuzzValidateID(f *testing.F) {
	for _, seed := range []string{
		"",
		strings.Repeat("a", 64),
		strings.Repeat("0", 64),
		strings.Repeat("A", 64),
		strings.Repeat("g", 64),
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		"zz" + strings.Repeat("a", 62),
		"\x00" + strings.Repeat("a", 63),
		"漢字" + strings.Repeat("a", 61),
		strings.Repeat("a", 63) + "?",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, id string) {
		t.Parallel()

		err := ValidateID(id)

		if err == nil {
			if len(id) != 64 {
				t.Fatalf("ValidateID(%q) = nil, want length error", id)
			}

			if _, decErr := hex.DecodeString(id); decErr != nil {
				t.Fatalf("ValidateID(%q) = nil, want non-hex error", id)
			}

			if strings.ToLower(id) != id {
				t.Fatalf("ValidateID(%q) = nil, want uppercase error", id)
			}
		}
	})
}
