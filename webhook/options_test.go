package webhook

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOptions_Validate_zero_valid(t *testing.T) {
	t.Parallel()
	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("zero Validate() = %v, want nil", err)
	}
}

func TestOptions_Validate_negativeTimeout_invalid(t *testing.T) {
	t.Parallel()
	err := Options{Timeout: -time.Second}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("negative timeout err = %v, want ErrInvalidOptions", err)
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("err %q missing timeout reason", err.Error())
	}
}

func TestOptions_Validate_negativeMaxRetries_invalid(t *testing.T) {
	t.Parallel()
	err := Options{MaxRetries: -1}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("negative maxretries err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "max retries") {
		t.Errorf("err %q missing max retries reason", err.Error())
	}
}

func TestOptions_Validate_bothNegative_joined(t *testing.T) {
	t.Parallel()
	err := Options{Timeout: -time.Second, MaxRetries: -1}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("joined err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "timeout") || !strings.Contains(err.Error(), "max retries") {
		t.Fatalf("joined err %q missing one of the reasons", err.Error())
	}
}
