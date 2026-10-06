package i18n

import (
	"errors"
	"testing"
)

func TestOptionsValidate_endpointWithoutScheme(t *testing.T) {
	t.Parallel()

	o := Options{}
	o.Remote.Endpoint = "example.com/x"

	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("endpoint without scheme err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptionsValidate_zeroTimeoutAndInflight_valid(t *testing.T) {
	t.Parallel()

	o := Options{Remote: RemoteOptions{Timeout: 0, MaxInFlight: 0}}
	if err := o.Validate(); err != nil {
		t.Fatalf("zero timeout/inflight Validate() = %v, want nil", err)
	}
}

func TestLocaleChain_whitespaceOnlyInputs_empty(t *testing.T) {
	t.Parallel()

	got := LocaleChain("   ", "  ")
	if len(got) != 0 {
		t.Fatalf("LocaleChain(whitespace) = %q, want empty", got)
	}
}

func TestOpen_invalidOptions(t *testing.T) {
	t.Parallel()

	got, err := Open(freshAdapter(), Options{Remote: RemoteOptions{Timeout: -1}})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open(invalid opts) err = %v, want ErrInvalidOptions", err)
	}

	var ioe InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not InvalidOptionsError", err)
	}

	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}
