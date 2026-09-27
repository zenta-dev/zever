package secrets

import (
	"testing"
)

func TestOptionsValidate_zero_returnsNil(t *testing.T) {
	t.Parallel()

	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
