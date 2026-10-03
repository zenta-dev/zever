package orm

import (
	"strings"
	"testing"
)

func TestNewCTENameErrorPrefix(t *testing.T) {
	for _, s := range []string{"", "2fast", "bad-name"} {
		_, err := NewCTEName(s)
		if err == nil {
			t.Fatalf("NewCTEName(%q) = nil error, want an error", s)
		}

		if !strings.HasPrefix(err.Error(), "orm:") {
			t.Fatalf("NewCTEName(%q) error = %q, want orm: prefix", s, err)
		}
	}
}
