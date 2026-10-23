package permission

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinels(t *testing.T) {
	t.Parallel()
	for _, e := range []error{ErrNilFactory, ErrDuplicate, ErrUnknownAdapter, ErrInvalidAdapter, ErrInvalidOptions} {
		if !strings.HasPrefix(e.Error(), "permission: ") {
			t.Errorf("sentinel %q missing %q prefix", e.Error(), "permission: ")
		}
	}
}

func TestErrors_typed_unwrap(t *testing.T) {
	t.Parallel()
	if !errors.Is(DuplicateAdapterError{Adapter: "rbac"}, ErrDuplicate) {
		t.Error("DuplicateAdapterError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(UnknownAdapterError{Adapter: "rbac"}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
}
