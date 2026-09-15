package webhook

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinel_messages(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"ErrNilFactory":       {ErrNilFactory.Error(), "webhook: nil factory"},
		"ErrDuplicateAdapter": {ErrDuplicateAdapter.Error(), "webhook: duplicate adapter"},
		"ErrUnknownAdapter":   {ErrUnknownAdapter.Error(), "webhook: unknown adapter"},
		"ErrInvalidAdapter":   {ErrInvalidAdapter.Error(), "webhook: invalid adapter"},
		"ErrInvalidOptions":   {ErrInvalidOptions.Error(), "webhook: invalid options"},
		"ErrNotFound":         {ErrNotFound.Error(), "webhook: not found"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q want %q", name, c[0], c[1])
		}
		if !strings.HasPrefix(c[0], "webhook: ") {
			t.Errorf("%s %q missing %q prefix", name, c[0], "webhook: ")
		}
	}
}

func TestErrors_typed_unwrap(t *testing.T) {
	t.Parallel()
	if !errors.Is(DuplicateAdapterError{Adapter: AdapterHTTP}, ErrDuplicateAdapter) {
		t.Error("DuplicateAdapterError does not unwrap to ErrDuplicateAdapter")
	}
	if !errors.Is(UnknownAdapterError{Adapter: AdapterHTTP}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	if de := (DuplicateAdapterError{Adapter: AdapterQueue}); de.Adapter != AdapterQueue {
		t.Errorf("DuplicateAdapterError adapter = %v", de.Adapter)
	}
	if ue := (UnknownAdapterError{Adapter: AdapterSQLite}); ue.Adapter != AdapterSQLite {
		t.Errorf("UnknownAdapterError adapter = %v", ue.Adapter)
	}
	if iae := (InvalidAdapterError{Adapter: "bogus"}); iae.Adapter != "bogus" {
		t.Errorf("InvalidAdapterError adapter = %q", iae.Adapter)
	}
	if ioe := (InvalidOptionsError{Reason: "bad"}); ioe.Reason != "bad" {
		t.Errorf("InvalidOptionsError reason = %q", ioe.Reason)
	}
}

func TestErrors_messages_prefixSentinel(t *testing.T) {
	t.Parallel()
	msgs := []string{
		(DuplicateAdapterError{Adapter: AdapterHTTP}).Error(),
		(UnknownAdapterError{Adapter: AdapterHTTP}).Error(),
		(InvalidAdapterError{Adapter: "bogus"}).Error(),
		(InvalidOptionsError{Reason: "bad"}).Error(),
	}
	for _, m := range msgs {
		if !strings.HasPrefix(m, "webhook: ") {
			t.Errorf("error %q missing %q prefix", m, "webhook: ")
		}
	}
}
