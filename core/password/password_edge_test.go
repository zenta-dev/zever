package password_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/password"
)

func TestOpen_invalidOptions_checkedBeforeLookup(t *testing.T) {
	t.Parallel()

	h, err := password.Open(password.Adapter("edge-unknown"), password.Options{})
	if h != nil {
		t.Fatal("Open(invalid opts) hasher != nil, want nil")
	}
	if !errors.Is(err, password.ErrInvalidHash) {
		t.Fatalf("Open(invalid opts, unknown adapter) err = %v, want ErrInvalidHash", err)
	}
}
