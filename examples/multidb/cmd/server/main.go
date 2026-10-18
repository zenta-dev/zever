// Command server runs the multi-database demo.
//
// Scaffolded by `zever new`. It is deliberately thin: serve.Main owns boot,
// signal handling and graceful shutdown.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/zenta-dev/zever/serve"

	"github.com/zenta-dev/zever/examples/multidb/internal/app"
)

func main() {
	flag.Parse()

	if err := serve.Main(app.Providers(), app.Modules()); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}
