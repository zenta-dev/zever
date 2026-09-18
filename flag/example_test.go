package flag_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/flag"
	"github.com/zenta-dev/zever/flag/static"
)

// ExampleOpen opens the static adapter and evaluates a missing key.
func ExampleOpen() {
	if err := flag.Register(flag.Static, static.New); err != nil {
		return
	}

	f, err := flag.Open(flag.Static, flag.Options{})
	if err != nil {
		return
	}

	defer func() { _ = f.Close() }()

	enabled, err := f.Bool(context.Background(), "new_checkout", true)
	if err != nil {
		return
	}

	fmt.Println(enabled)
	// Output: true
}
