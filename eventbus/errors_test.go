package eventbus

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinel_messages(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"ErrClosed":           {ErrClosed.Error(), "eventbus: closed"},
		"ErrNilFactory":       {ErrNilFactory.Error(), "eventbus: nil factory"},
		"ErrDuplicate":        {ErrDuplicate.Error(), "eventbus: duplicate registration"},
		"ErrUnknownAdapter":   {ErrUnknownAdapter.Error(), "eventbus: unknown adapter"},
		"ErrInvalidAdapter":   {ErrInvalidAdapter.Error(), "eventbus: invalid adapter"},
		"ErrInvalidOptions":   {ErrInvalidOptions.Error(), "eventbus: invalid options"},
		"ErrNilHandler":       {ErrNilHandler.Error(), "eventbus: nil handler"},
		"ErrPayloadTooLarge":  {ErrPayloadTooLarge.Error(), "eventbus: payload too large"},
		"ErrInvalidMessageID": {ErrInvalidMessageID.Error(), "eventbus: invalid message id"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q want %q", name, c[0], c[1])
		}
		if !strings.HasPrefix(c[0], "eventbus: ") {
			t.Errorf("%s %q missing %q prefix", name, c[0], "eventbus: ")
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
	cause := errors.New("parse boom")
	if !errors.Is(&InvalidMessageIDError{ID: "x", Err: cause}, cause) {
		t.Error("InvalidMessageIDError does not unwrap to cause")
	}
	if !errors.Is(&InvalidMessageIDError{ID: "x", Err: cause}, ErrInvalidMessageID) {
		t.Error("InvalidMessageIDError does not unwrap to ErrInvalidMessageID")
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
	cause := errors.New("boom")
	if ime := (&InvalidMessageIDError{ID: "bad", Err: cause}); ime.ID != "bad" || !errors.Is(ime.Err, cause) {
		t.Errorf("InvalidMessageIDError fields = %+v", ime)
	}
}

func TestParseMessageIDInvalid(t *testing.T) {
	t.Parallel()
	_, err := ParseMessageID("nope")
	if err == nil {
		t.Fatal("ParseMessageID = nil error, want invalid ID error")
	}
	if !errors.Is(err, ErrInvalidMessageID) {
		t.Errorf("errors.Is(err, ErrInvalidMessageID) = false (err = %v)", err)
	}
	var ime *InvalidMessageIDError
	if !errors.As(err, &ime) {
		t.Fatalf("errors.As(err, InvalidMessageIDError) = false (err = %T %v)", err, err)
	}
	if ime.ID != "nope" {
		t.Errorf("InvalidMessageIDError.ID = %q, want nope", ime.ID)
	}
}
