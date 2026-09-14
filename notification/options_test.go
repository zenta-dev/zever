package notification

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

func TestOptions_Validate_negativeTimeout_invalid(t *testing.T) {
	t.Parallel()
	o := Options{Timeout: -time.Second}
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("negative timeout err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_defaults_passthrough(t *testing.T) {
	t.Parallel()
	o := Options{Timeout: 5 * time.Second}
	if o.timeout() != 5*time.Second {
		t.Errorf("timeout() = %v want %v", o.timeout(), 5*time.Second)
	}
}
