package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinel_messages(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"ErrClosed":         {ErrClosed.Error(), "i18n: closed"},
		"ErrNilFactory":     {ErrNilFactory.Error(), "i18n: nil factory"},
		"ErrDuplicate":      {ErrDuplicate.Error(), "i18n: duplicate adapter"},
		"ErrUnknownAdapter": {ErrUnknownAdapter.Error(), "i18n: unknown adapter"},
		"ErrInvalidAdapter": {ErrInvalidAdapter.Error(), "i18n: invalid adapter"},
		"ErrInvalidOptions": {ErrInvalidOptions.Error(), "i18n: invalid options"},
		"ErrLocaleNotFound": {ErrLocaleNotFound.Error(), "i18n: locale not found"},
		"ErrKeyNotFound":    {ErrKeyNotFound.Error(), "i18n: key not found"},
		"ErrRemoteError":    {ErrRemoteError.Error(), "i18n: remote error"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q want %q", name, c[0], c[1])
		}
		if !strings.HasPrefix(c[0], "i18n: ") {
			t.Errorf("%s %q missing %q prefix", name, c[0], "i18n: ")
		}
	}
}

func TestErrors_typed_unwrap(t *testing.T) {
	t.Parallel()
	if !errors.Is(DuplicateAdapterError{Adapter: "embed"}, ErrDuplicate) {
		t.Error("DuplicateAdapterError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(UnknownAdapterError{Adapter: "embed"}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
	if !errors.Is(LocaleNotFoundError{Locale: "x"}, ErrLocaleNotFound) {
		t.Error("LocaleNotFoundError does not unwrap to ErrLocaleNotFound")
	}
	if !errors.Is(KeyNotFoundError{Locale: "x", Key: "y"}, ErrKeyNotFound) {
		t.Error("KeyNotFoundError does not unwrap to ErrKeyNotFound")
	}
}
