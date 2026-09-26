package cache_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/cache"
	"github.com/zenta-dev/zever/cache/memory"
)

// ExampleOpen opens the memory backend, stores a value and reads it back.
func ExampleOpen() {
	backend, err := memory.New(cache.Options{})
	if err != nil {
		fmt.Println("open error")
		return
	}
	defer backend.Close(context.Background())

	ctx := context.Background()
	if setErr := backend.Set(ctx, "greeting", []byte("hello"), 0); setErr != nil {
		fmt.Println("set error")
		return
	}

	value, err := backend.Get(ctx, "greeting")
	if err != nil {
		fmt.Println("get error")
		return
	}

	fmt.Println(string(value))
	// Output: hello
}
