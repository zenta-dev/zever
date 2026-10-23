package i18n

import (
	"errors"
	"testing"
	"time"
)

func TestOptions_Validate_urlParseFailure_invalid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.Endpoint = "100%"
	err := o.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("url parse failure err = %v, want ErrInvalidOptions", err)
	}
	if want := "i18n: invalid options: endpoint must be a valid url"; err.Error() != want {
		t.Errorf("url parse failure err %q, want %q", err.Error(), want)
	}
	var ioe InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Errorf("url parse failure err %T is not *InvalidOptionsError", err)
	} else if ioe.Reason != "endpoint must be a valid url" {
		t.Errorf("url parse failure reason %q", ioe.Reason)
	}
}

func TestOptions_Validate_httpsEndpoint_valid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.Endpoint = "https://example.com/x"
	if err := o.Validate(); err != nil {
		t.Errorf("https Validate() = %v, want nil", err)
	}
}

func TestOptions_timeout_defaultAndOverride(t *testing.T) {
	t.Parallel()
	zero := Options{}
	if got := zero.timeout(); got != DefaultTimeout {
		t.Errorf("zero timeout() = %v, want %v", got, DefaultTimeout)
	}
	override := Options{}
	override.Remote.Timeout = 5 * time.Second
	if got := override.timeout(); got != 5*time.Second {
		t.Errorf("explicit timeout() = %v, want %v", got, 5*time.Second)
	}
}
