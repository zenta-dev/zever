// Command gengrammar regenerates the editor grammar files
// (editors/nvim/syntax/zen.vim and editors/vscode/syntaxes/zen.tmLanguage.json)
// from the DSL compiler's own keyword and scalar-type tables. It is a thin
// wrapper around internal/dsl/gengrammar.Files; all the actual generation
// logic lives there so editors/parity_test.go can call it directly instead
// of shelling out.
//
// Run via `make generate`, or directly with `go run ./tools/gengrammar`
// from the repository root.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zenta-dev/zever/internal/dsl/gengrammar"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gengrammar:", err)
		os.Exit(1)
	}
}

// run writes every generated file returned by gengrammar.Files() to its
// path relative to the current working directory, which must be the
// repository root.
func run() error {
	for path, content := range gengrammar.Files() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", path, err)
		}

		if err := os.WriteFile(path, content, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	return nil
}
