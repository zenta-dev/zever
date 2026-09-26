package redis

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/ratelimit"
)

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(ratelimit.Options{Rate: -1})
	if err == nil {
		t.Fatal("New() = nil error, want invalid options error")
	}
	if !errors.Is(err, ratelimit.ErrInvalidOptions) {
		t.Errorf("errors.Is(err, ErrInvalidOptions) = false (err = %v)", err)
	}
	if !strings.HasPrefix(err.Error(), "redis: ") {
		t.Errorf("error %q missing %q prefix", err.Error(), "redis: ")
	}
}
