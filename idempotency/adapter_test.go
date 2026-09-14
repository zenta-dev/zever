package idempotency

import (
	"errors"
	"strings"
	"testing"
)

func TestAdapter_String_memory(t *testing.T) {
	t.Parallel()
	if got := Memory.String(); got != "memory" {
		t.Fatalf("Memory.String() = %q, want %q", got, "memory")
	}
}

func TestAdapter_String_redis(t *testing.T) {
	t.Parallel()
	if got := Redis.String(); got != "redis" {
		t.Fatalf("Redis.String() = %q, want %q", got, "redis")
	}
}

func TestAdapter_String_unknown(t *testing.T) {
	t.Parallel()
	if got := Adapter(99).String(); got != "unknown" {
		t.Fatalf("Adapter(99).String() = %q, want %q", got, "unknown")
	}
}

func TestParseAdapter_memory_success(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("memory")
	if err != nil {
		t.Fatalf("ParseAdapter(memory) err = %v", err)
	}
	if a != Memory {
		t.Fatalf("ParseAdapter(memory) = %v, want Memory", a)
	}
}

func TestParseAdapter_redis_success(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("redis")
	if err != nil {
		t.Fatalf("ParseAdapter(redis) err = %v", err)
	}
	if a != Redis {
		t.Fatalf("ParseAdapter(redis) = %v, want Redis", a)
	}
}

func TestParseAdapter_invalid_uppercase_fails(t *testing.T) {
	t.Parallel()
	_, err := ParseAdapter("Memory")
	if err == nil {
		t.Fatal("ParseAdapter(Memory) expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidAdapter) {
		t.Fatalf("ParseAdapter(Memory) err = %v, want ErrInvalidAdapter", err)
	}
}

func TestParseAdapter_invalid_empty_fails(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("")
	if err == nil {
		t.Fatal("ParseAdapter empty expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidAdapter) {
		t.Fatalf("ParseAdapter empty err = %v, want ErrInvalidAdapter", err)
	}
	if a != Memory {
		t.Fatalf("ParseAdapter empty adapter = %v, want Memory zero value", a)
	}
}

func TestParseAdapter_invalid_unknown_fails(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("postgres")
	if err == nil {
		t.Fatal("ParseAdapter(postgres) expected error, got nil")
	}
	var iae *InvalidAdapterError
	if !errors.As(err, &iae) {
		t.Fatalf("ParseAdapter(postgres) err type = %T, want *InvalidAdapterError", err)
	}
	if iae.Adapter != "postgres" {
		t.Fatalf("InvalidAdapterError.Adapter = %q, want %q", iae.Adapter, "postgres")
	}
	if a != Memory {
		t.Fatalf("ParseAdapter(postgres) adapter = %v, want Memory zero value", a)
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("error message should carry adapter name, got %q", err.Error())
	}
}
