package mailer

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinel_messages(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"ErrClosed":          {ErrClosed.Error(), "mailer: closed"},
		"ErrNilFactory":      {ErrNilFactory.Error(), "mailer: nil factory"},
		"ErrDuplicate":       {ErrDuplicate.Error(), "mailer: duplicate registration"},
		"ErrUnknownAdapter":  {ErrUnknownAdapter.Error(), "mailer: unknown adapter"},
		"ErrInvalidAdapter":  {ErrInvalidAdapter.Error(), "mailer: invalid adapter"},
		"ErrInvalidOptions":  {ErrInvalidOptions.Error(), "mailer: invalid options"},
		"ErrInvalidAddress":  {ErrInvalidAddress.Error(), "mailer: invalid address"},
		"ErrNoRecipients":    {ErrNoRecipients.Error(), "mailer: no recipients"},
		"ErrMessageTooLarge": {ErrMessageTooLarge.Error(), "mailer: message too large"},
		"ErrNilMessage":      {ErrNilMessage.Error(), "mailer: nil message"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q want %q", name, c[0], c[1])
		}
		if !strings.HasPrefix(c[0], "mailer: ") {
			t.Errorf("%s %q missing %q prefix", name, c[0], "mailer: ")
		}
	}
}

func TestErrors_typed_unwrap(t *testing.T) {
	t.Parallel()
	if !errors.Is(&DuplicateError{Adapter: SMTP}, ErrDuplicate) {
		t.Error("DuplicateError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(&UnknownAdapterError{Adapter: SMTP}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(&InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(&InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
	if !errors.Is(&InvalidAddressError{Field: "To", Value: "x"}, ErrInvalidAddress) {
		t.Error("InvalidAddressError does not unwrap to ErrInvalidAddress")
	}
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	if de := (&DuplicateError{Adapter: SMTP}); de.Adapter != SMTP {
		t.Errorf("DuplicateError adapter = %v", de.Adapter)
	}
	if ue := (&UnknownAdapterError{Adapter: Log}); ue.Adapter != Log {
		t.Errorf("UnknownAdapterError adapter = %v", ue.Adapter)
	}
	if iae := (&InvalidAdapterError{Adapter: "bogus"}); iae.Adapter != "bogus" {
		t.Errorf("InvalidAdapterError adapter = %q", iae.Adapter)
	}
	if ioe := (&InvalidOptionsError{Reason: "bad"}); ioe.Reason != "bad" {
		t.Errorf("InvalidOptionsError reason = %q", ioe.Reason)
	}
	if iae := (&InvalidAddressError{Field: "From", Value: "bad"}); iae.Field != "From" || iae.Value != "bad" {
		t.Errorf("InvalidAddressError fields = %+v", iae)
	}
}
