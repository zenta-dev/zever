package registry_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/shared/registry"
)

// TestRegister_typedNilFactories verifies isNilFactory catches typed nils for
// pointer, map, and channel factory types, not just funcs.
func TestRegister_typedNilFactories(t *testing.T) {
	t.Parallel()

	var p *int
	rPtr := registry.New[string, *int](errNil, nil, nil)
	if err := rPtr.Register("a", p); !errors.Is(err, errNil) {
		t.Errorf("Register(typed-nil pointer) = %v, want %v", err, errNil)
	}

	var m map[string]int
	rMap := registry.New[string, map[string]int](errNil, nil, nil)
	if err := rMap.Register("a", m); !errors.Is(err, errNil) {
		t.Errorf("Register(typed-nil map) = %v, want %v", err, errNil)
	}

	var ch chan int
	rChan := registry.New[string, chan int](errNil, nil, nil)
	if err := rChan.Register("a", ch); !errors.Is(err, errNil) {
		t.Errorf("Register(typed-nil chan) = %v, want %v", err, errNil)
	}
}
