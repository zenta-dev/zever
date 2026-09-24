package env

import (
	"strings"
	"testing"
)

func TestOptionsValidate_joinsViolations(t *testing.T) {
	t.Parallel()

	err := Options{Prefix: "a/b c"}.Validate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	for _, want := range []string{"slash or dot-dot", "spaces"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}

	if err := (Options{Prefix: "OK_"}).Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
