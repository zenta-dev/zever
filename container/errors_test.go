package container

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

var (
	_ error = TransactorError{}
	_ error = CloseTimeoutError{}
	_ error = ClosePanicError{}
)

func TestSentinels_ContainerPrefix(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"transactor unsupported", ErrTransactorUnsupported},
		{"close timeout", ErrCloseTimeout},
		{"close panic", ErrClosePanic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.HasPrefix(tt.err.Error(), "container:") {
				t.Fatalf("sentinel message = %q, want \"container:\" prefix", tt.err.Error())
			}
			if !errors.Is(tt.err, tt.err) {
				t.Fatalf("errors.Is(%v, itself) is false", tt.err)
			}
		})
	}
}

func TestTransactorError(t *testing.T) {
	err := TransactorError{Actual: "sqlite"}

	want := "container: sqlite does not support transactions"
	if err.Error() != want {
		t.Fatalf("Error() = %q, want %q", err.Error(), want)
	}
	if u := err.Unwrap(); !errors.Is(u, ErrTransactorUnsupported) {
		t.Fatalf("Unwrap() = %v, want %v", u, ErrTransactorUnsupported)
	}
	if !errors.Is(err, ErrTransactorUnsupported) {
		t.Fatal("errors.Is(err, ErrTransactorUnsupported) is false")
	}
	if errors.Is(err, ErrCloseTimeout) || errors.Is(err, ErrClosePanic) {
		t.Fatal("errors.Is matches an unrelated sentinel")
	}

	var as TransactorError
	if !errors.As(err, &as) {
		t.Fatal("errors.As failed to match TransactorError")
	}
	if as.Actual != "sqlite" {
		t.Fatalf("errors.As Actual = %q, want %q", as.Actual, "sqlite")
	}
}

func TestCloseTimeoutError(t *testing.T) {
	inner := fmt.Errorf("dial tcp: %w", context.DeadlineExceeded)
	err := CloseTimeoutError{Service: "db", Err: inner}

	if !strings.Contains(err.Error(), "db") {
		t.Fatalf("Error() = %q, want it to name the service", err.Error())
	}
	if !strings.HasPrefix(err.Error(), "container:") {
		t.Fatalf("Error() = %q, want \"container:\" prefix", err.Error())
	}
	if u := err.Unwrap(); !errors.Is(u, inner) {
		t.Fatalf("Unwrap() = %v, want the stored error", u)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("errors.Is(err, context.DeadlineExceeded) is false")
	}
	if errors.Is(err, ErrClosePanic) {
		t.Fatal("errors.Is matches an unrelated sentinel")
	}

	var as CloseTimeoutError
	if !errors.As(err, &as) {
		t.Fatal("errors.As failed to match CloseTimeoutError")
	}
	if as.Service != "db" {
		t.Fatalf("errors.As Service = %q, want %q", as.Service, "db")
	}
}

func TestClosePanicError(t *testing.T) {
	err := ClosePanicError{Service: "cache", Panic: "boom"}

	if !strings.Contains(err.Error(), "cache") {
		t.Fatalf("Error() = %q, want it to name the service", err.Error())
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Error() = %q, want it to carry the panic value", err.Error())
	}
	if u := err.Unwrap(); !errors.Is(u, ErrClosePanic) {
		t.Fatalf("Unwrap() = %v, want %v", u, ErrClosePanic)
	}
	if !errors.Is(err, ErrClosePanic) {
		t.Fatal("errors.Is(err, ErrClosePanic) is false")
	}
	if errors.Is(err, ErrCloseTimeout) {
		t.Fatal("errors.Is matches an unrelated sentinel")
	}

	var as ClosePanicError
	if !errors.As(err, &as) {
		t.Fatal("errors.As failed to match ClosePanicError")
	}
	if as.Service != "cache" || as.Panic != "boom" {
		t.Fatalf("errors.As = %+v, want {cache boom}", as)
	}
}
