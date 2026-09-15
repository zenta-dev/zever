package flag

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
}

func TestOptions_Validate_negativeFirebaseTimeout_invalid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Firebase.Timeout = -time.Second
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("negative firebase timeout err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_emptyStaticPath_valid(t *testing.T) {
	t.Parallel()
	o := Options{Static: StaticOptions{Path: ""}}
	if err := o.Validate(); err != nil {
		t.Fatalf("empty static path Validate() = %v, want nil", err)
	}
}

func TestOptions_Validate_zeroFirebaseTimeout_valid(t *testing.T) {
	t.Parallel()
	o := Options{Firebase: FirebaseOptions{Timeout: 0}}
	if err := o.Validate(); err != nil {
		t.Fatalf("zero firebase timeout Validate() = %v, want nil", err)
	}
	if DefaultTimeout != 30*time.Second {
		t.Errorf("DefaultTimeout = %v want %v", DefaultTimeout, 30*time.Second)
	}
}
