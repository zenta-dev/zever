package sms_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/examples/sms"
	"github.com/zenta-dev/zever/examples/sms/stub"
)

// TestOptionsValidate pins the sender-identity requirement.
func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	if err := (sms.Options{}).Validate(); !errors.Is(err, sms.ErrInvalidOptions) {
		t.Fatalf("Options{}.Validate() = %v, want ErrInvalidOptions", err)
	}

	if err := (sms.Options{From: "+15550000"}).Validate(); err != nil {
		t.Fatalf("Options{From}.Validate() = %v, want nil", err)
	}
}

// TestInvalidOptionsError pins the typed error: the sentinel, the sms:
// prefix, and the reason text.
func TestInvalidOptionsError(t *testing.T) {
	t.Parallel()

	err := &sms.InvalidOptionsError{Reason: "from must be non-empty"}

	if !errors.Is(err, sms.ErrInvalidOptions) {
		t.Fatalf("InvalidOptionsError = %v, want ErrInvalidOptions", err)
	}

	if !strings.Contains(err.Error(), "sms: invalid options") {
		t.Fatalf("InvalidOptionsError.Error() = %q, want sms: prefix", err.Error())
	}

	if !strings.Contains(err.Error(), "from must be non-empty") {
		t.Fatalf("InvalidOptionsError.Error() = %q, want reason", err.Error())
	}
}

// TestConstructorShape pins the code-wiring contract: the stub backend
// constructor satisfies sms.Factory, so apps wire it with
// container.Provide instead of a name-based open.
func TestConstructorShape(t *testing.T) {
	t.Parallel()

	var factory sms.Factory = stub.Open

	svc, err := factory(sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}

	if svc == nil {
		t.Fatal("factory returned nil SMS")
	}

	if err := svc.Send(t.Context(), "+15550001", "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if err := svc.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// BenchmarkOptionsValidate measures options validation on the config load
// path.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := sms.Options{From: "+15550000"}

	b.ReportAllocs()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate: %v", err)
		}
	}
}
