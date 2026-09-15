package flag

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinel_messages(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"ErrClosed":         {ErrClosed.Error(), "flag: closed"},
		"ErrNilFactory":     {ErrNilFactory.Error(), "flag: nil factory"},
		"ErrDuplicate":      {ErrDuplicate.Error(), "flag: duplicate registration"},
		"ErrUnknownAdapter": {ErrUnknownAdapter.Error(), "flag: unknown adapter"},
		"ErrInvalidAdapter": {ErrInvalidAdapter.Error(), "flag: invalid adapter"},
		"ErrInvalidOptions": {ErrInvalidOptions.Error(), "flag: invalid options"},
		"ErrInvalidKey":     {ErrInvalidKey.Error(), "flag: invalid key"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q want %q", name, c[0], c[1])
		}
		if !strings.HasPrefix(c[0], "flag: ") {
			t.Errorf("%s %q missing %q prefix", name, c[0], "flag: ")
		}
	}
}

func TestErrors_typed_unwrap(t *testing.T) {
	t.Parallel()
	if !errors.Is(&DuplicateError{Adapter: Static}, ErrDuplicate) {
		t.Error("DuplicateError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(&UnknownAdapterError{Adapter: Static}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(&InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(&InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
	if !errors.Is(&InvalidKeyError{KeyLen: 0}, ErrInvalidKey) {
		t.Error("InvalidKeyError does not unwrap to ErrInvalidKey")
	}
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	if de := (&DuplicateError{Adapter: Static}); de.Adapter != Static {
		t.Errorf("DuplicateError adapter = %v", de.Adapter)
	}
	if ue := (&UnknownAdapterError{Adapter: Static}); ue.Adapter != Static {
		t.Errorf("UnknownAdapterError adapter = %v", ue.Adapter)
	}
	if iae := (&InvalidAdapterError{Adapter: "bogus"}); iae.Adapter != "bogus" {
		t.Errorf("InvalidAdapterError adapter = %q", iae.Adapter)
	}
	if ioe := (&InvalidOptionsError{Reason: "bad"}); ioe.Reason != "bad" {
		t.Errorf("InvalidOptionsError reason = %q", ioe.Reason)
	}
	if ike := (&InvalidKeyError{KeyLen: 5}); ike.KeyLen != 5 {
		t.Errorf("InvalidKeyError keylen = %d", ike.KeyLen)
	}
}
