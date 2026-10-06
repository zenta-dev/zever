package resilience_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/resilience"
)

// ExampleDo runs a typed operation through a guard.
func ExampleDo() {
	g := &stubGuard{name: "dep"}

	v, err := resilience.Do(context.Background(), g, func(context.Context) (string, error) {
		return "ok", nil
	})
	if err != nil {
		return
	}

	fmt.Println(v)
	// Output: ok
}

// ExampleParseAdapter parses a custom adapter name.
func ExampleParseAdapter() {
	a, err := resilience.ParseAdapter("memory")
	if err != nil {
		return
	}

	fmt.Println(a)
	// Output: memory
}
