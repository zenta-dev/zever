package main

import (
	"github.com/zenta-dev/zever/container/adapters"
)

// Adapters in this binary are registered explicitly: adapter packages
// expose constructors but never self-register, so blank imports alone would
// resolve nothing. The container wires light adapters itself; the
// heavyweight families live in container/adapters and are registered here,
// once, at startup (the composition root). Registration performs no I/O.
func init() {
	adapters.RegisterAll()
}
