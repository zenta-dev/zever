package providersopt

import (
	"errors"
	"testing"
)

// TestValidateEndpoint_schemeOnly verifies a scheme without a host is rejected.
func TestValidateEndpoint_schemeOnly(t *testing.T) {
	t.Parallel()

	errs := ValidateEndpoint("https://")
	if len(errs) == 0 {
		t.Fatal("ValidateEndpoint(https://) = nil, want host error")
	}

	if !errors.Is(errs[0], ErrInvalidEndpoint) {
		t.Errorf("err = %v, want ErrInvalidEndpoint", errs[0])
	}
}
