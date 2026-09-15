package i18n

import (
	"errors"
	"testing"
	"time"
)

func TestOptions_Validate_zero_valid(t *testing.T) {
	t.Parallel()
	o := Options{}
	if err := o.Validate(); err != nil {
		t.Fatalf("zero Validate() = %v, want nil", err)
	}
	if o.timeout() != DefaultTimeout {
		t.Errorf("timeout() = %v want %v", o.timeout(), DefaultTimeout)
	}
}

func TestOptions_Validate_badEndpointScheme_invalid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.Endpoint = "ftp://example.com/x"
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("bad scheme err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_endpointWithoutHost_invalid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.Endpoint = "https://"
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("no-host err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_httpWithoutInsecure_invalid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.Endpoint = "http://example.com/x"
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("http without insecure err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_httpWithInsecure_valid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.Endpoint = "http://example.com/x"
	o.Remote.AllowInsecure = true
	if err := o.Validate(); err != nil {
		t.Errorf("http with insecure Validate() = %v, want nil", err)
	}
}

func TestOptions_Validate_negativeTimeout_invalid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.Timeout = -time.Second
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("negative timeout err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_negativeMaxInFlight_invalid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.MaxInFlight = -1
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("negative inflight err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_defaults_passthrough(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.Timeout = 5 * time.Second
	if o.timeout() != 5*time.Second {
		t.Errorf("timeout() = %v want %v", o.timeout(), 5*time.Second)
	}
}
