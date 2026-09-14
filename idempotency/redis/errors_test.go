package redis

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/idempotency"
)

func TestDecodeCorruptIs(t *testing.T) {
	t.Parallel()

	cases := map[string][]byte{
		"empty":     {},
		"short":     []byte("x"),
		"bad tag":   {'X', 0, 0},
		"truncated": {tagPending, 0, 10, 'a'},
	}

	for name, raw := range cases {
		if _, _, _, err := decode(raw); err == nil {
			t.Errorf("decode(%s) = nil error, want corrupt record", name)
		} else if !errors.Is(err, idempotency.ErrCorruptRecord) {
			t.Errorf("decode(%s) err = %v, want ErrCorruptRecord", name, err)
		}
	}
}

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(idempotency.Options{TTL: -1})
	if err == nil {
		t.Fatal("New() = nil error, want invalid options error")
	}
	if !errors.Is(err, idempotency.ErrInvalidOptions) {
		t.Errorf("errors.Is(err, ErrInvalidOptions) = false (err = %v)", err)
	}
	if !strings.HasPrefix(err.Error(), "redis: ") {
		t.Errorf("error %q missing %q prefix", err.Error(), "redis: ")
	}
}
