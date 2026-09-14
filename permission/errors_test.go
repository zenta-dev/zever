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
	if !errors.Is(&DuplicateError{Adapter: RBAC}, ErrDuplicate) {
		t.Error("DuplicateError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(&UnknownAdapterError{Adapter: RBAC}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(&InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(&InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	de := &DuplicateError{Adapter: Casbin}
	if de.Adapter != Casbin {
		t.Errorf("DuplicateError adapter = %v", de.Adapter)
	}
	ue := &UnknownAdapterError{Adapter: RBAC}
	if ue.Adapter != RBAC {
		t.Errorf("UnknownAdapterError adapter = %v", ue.Adapter)
	}
	iae := &InvalidAdapterError{Adapter: "bogus"}
	if iae.Adapter != "bogus" {
		t.Errorf("InvalidAdapterError adapter = %q", iae.Adapter)
	}
	ioe := &InvalidOptionsError{Reason: "bad"}
	if ioe.Reason != "bad" {
		t.Errorf("InvalidOptionsError reason = %q", ioe.Reason)
	}
}
