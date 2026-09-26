package session

import (
	"errors"
	"testing"
)

func TestAdapter_String_memory(t *testing.T) {
	t.Parallel()
	if got := Memory.String(); got != "memory" {
		t.Fatalf("Memory.String() = %q, want %q", got, "memory")
	}
}

func TestAdapter_String_unknown(t *testing.T) {
	t.Parallel()
	if got := Adapter("").String(); got != "unknown" {
		t.Fatalf("Adapter(empty).String() = %q, want %q", got, "unknown")
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

func TestParseAdapter_invalid_uppercase_fails(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("Memory")
	if err != nil {
		t.Fatalf("ParseAdapter(Memory) err = %v, want nil (open adapter)", err)
	}
	if a != Adapter("Memory") {
		t.Fatalf("ParseAdapter(Memory) = %v, want %v", a, Adapter("Memory"))
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
	if a != Adapter("") {
		t.Fatalf("ParseAdapter empty adapter = %v, want empty zero value", a)
	}
}

func TestParseAdapter_invalid_unknown_fails(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("postgres")
	if err != nil {
		t.Fatalf("ParseAdapter(postgres) err = %v, want nil (open adapter)", err)
	}
	if a != Adapter("postgres") {
		t.Fatalf("ParseAdapter(postgres) adapter = %v, want %v", a, Adapter("postgres"))
	}
}
