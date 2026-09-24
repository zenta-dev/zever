package lock

import (
	"testing"
	"time"
)

func TestOptionsValidate_zero_returnsNil(t *testing.T) {
	t.Parallel()

	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDefaults_values(t *testing.T) {
	t.Parallel()

	if DefaultTTL != 30*time.Second {
		t.Errorf("DefaultTTL = %v, want 30s", DefaultTTL)
	}

	if DefaultRetryInterval != 50*time.Millisecond {
		t.Errorf("DefaultRetryInterval = %v, want 50ms", DefaultRetryInterval)
	}
}
