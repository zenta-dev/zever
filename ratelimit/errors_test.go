package ratelimit

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinel_messages(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"ErrNilFactory":     {ErrNilFactory.Error(), "ratelimit: nil factory"},
		"ErrDuplicate":      {ErrDuplicate.Error(), "ratelimit: duplicate registration"},
		"ErrUnknownAdapter": {ErrUnknownAdapter.Error(), "ratelimit: unknown adapter"},
		"ErrInvalidAdapter": {ErrInvalidAdapter.Error(), "ratelimit: invalid adapter"},
		"ErrInvalidOptions": {ErrInvalidOptions.Error(), "ratelimit: invalid options"},
		"ErrInvalidKey":     {ErrInvalidKey.Error(), "ratelimit: invalid key"},
		"ErrInvalidCost":    {ErrInvalidCost.Error(), "ratelimit: invalid cost"},
		"ErrClosed":         {ErrClosed.Error(), "ratelimit: closed"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q want %q", name, c[0], c[1])
		}
		if !strings.HasPrefix(c[0], "ratelimit: ") {
			t.Errorf("%s %q missing %q prefix", name, c[0], "ratelimit: ")
		}
	}
}

func TestErrors_typed_unwrap(t *testing.T) {
	t.Parallel()
	if !errors.Is(&DuplicateError{Adapter: Redis}, ErrDuplicate) {
		t.Error("DuplicateError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(&UnknownAdapterError{Adapter: Redis}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(&InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(&InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
	if !errors.Is(&InvalidKeyError{KeyLen: 3}, ErrInvalidKey) {
		t.Error("InvalidKeyError does not unwrap to ErrInvalidKey")
	}
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	if de := (&DuplicateError{Adapter: Redis}); de.Adapter != Redis {
		t.Errorf("DuplicateError adapter = %v", de.Adapter)
	}
	if ue := (&UnknownAdapterError{Adapter: Memory}); ue.Adapter != Memory {
		t.Errorf("UnknownAdapterError adapter = %v", ue.Adapter)
	}
	if iae := (&InvalidAdapterError{Adapter: "bogus"}); iae.Adapter != "bogus" {
		t.Errorf("InvalidAdapterError adapter = %q", iae.Adapter)
	}
	if ioe := (&InvalidOptionsError{Reason: "bad"}); ioe.Reason != "bad" {
		t.Errorf("InvalidOptionsError reason = %q", ioe.Reason)
	}
	if ike := (&InvalidKeyError{KeyLen: 300}); ike.KeyLen != 300 {
		t.Errorf("InvalidKeyError keylen = %d", ike.KeyLen)
	}
}
