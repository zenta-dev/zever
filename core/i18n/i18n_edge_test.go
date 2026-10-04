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
