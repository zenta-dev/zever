package ollama

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrNoModel_message(t *testing.T) {
	t.Parallel()

	if got := ErrNoModel.Error(); got != "ollama: model is required" {
		t.Fatalf("Error() = %q", got)
	}

	wrapped := fmt.Errorf("generate: %w", ErrNoModel)
	if !errors.Is(wrapped, ErrNoModel) {
		t.Fatalf("errors.Is(%v, ErrNoModel) = false", wrapped)
	}
}
