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

func TestOptionScanUnsupportedTypeErrorPrefix(t *testing.T) {
	var o Option[chan int]

	err := o.Scan("x")
	if err == nil {
		t.Fatal("Scan into Option[chan int] = nil error, want an error")
	}

	if !strings.HasPrefix(err.Error(), "orm:") {
		t.Fatalf("Scan error = %q, want orm: prefix", err)
	}
}
