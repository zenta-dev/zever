// Command server runs the showcase shop HTTP+gRPC API.
//
// Scaffolded by `zever new`. It is deliberately thin: serve.Main owns boot,
// signal handling and graceful shutdown; everything with behaviour worth
// testing belongs in a package of its own.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/zenta-dev/zever/serve"

	"github.com/zenta-dev/zever/examples/showcase/internal/app"
)

func main() {
	flag.Parse()

	if err := serve.Main(app.Providers(), app.Modules()); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}
