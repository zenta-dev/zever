package container

import (
	"errors"
	"testing"
)

// TestContainer_EventbusAlias pins that the Eventbus compatibility alias
// resolves the same singleton as EventBus.
func TestContainer_EventbusAlias(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	alias, err := c.Eventbus()
	if err != nil {
		t.Fatalf("Eventbus: %v", err)
	}

	canonical, err := c.EventBus()
	if err != nil {
		t.Fatalf("EventBus: %v", err)
	}

	sameInstance(t, alias, canonical)
}

// TestContainer_RatelimitAlias pins that the Ratelimit compatibility alias
// resolves the same singleton as RateLimit.
func TestContainer_RatelimitAlias(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	alias, err := c.Ratelimit()
	if err != nil {
		t.Fatalf("Ratelimit: %v", err)
	}

	canonical, err := c.RateLimit()
	if err != nil {
		t.Fatalf("RateLimit: %v", err)
	}

	sameInstance(t, alias, canonical)
}

// TestCloseTimeoutError_UnwrapNilErr covers the no-underlying-error branch:
// Unwrap still yields ErrCloseTimeout so errors.Is keeps matching.
func TestCloseTimeoutError_UnwrapNilErr(t *testing.T) {
	t.Parallel()

	e := CloseTimeoutError{Service: "cache"}
	if !errors.Is(e, ErrCloseTimeout) {
		t.Fatalf("errors.Is(%v, ErrCloseTimeout) = false, want true", e)
	}
	if !errors.Is(e.Unwrap(), ErrCloseTimeout) {
		t.Fatalf("Unwrap = %v, want ErrCloseTimeout", e.Unwrap())
	}
}

// TestClosePanicError_Unwrap covers the panic wrapper's sentinel match and
// message.
func TestClosePanicError_Unwrap(t *testing.T) {
	t.Parallel()

	e := ClosePanicError{Service: "db", Panic: "boom"}
	if !errors.Is(e, ErrClosePanic) {
		t.Fatalf("errors.Is(%v, ErrClosePanic) = false, want true", e)
	}
	if got := e.Error(); got == "" {
		t.Fatal("Error() = empty")
	}
}
