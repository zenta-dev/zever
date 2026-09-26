package main

import (
	"fmt"
	"io"
	"os"

	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// BoundariesConfig carries every input runCheckBoundariesWith needs. Screen
// agents build it from huh forms; the flag shell (runCheckBoundaries)
// builds it from argv. A nil Out defaults to os.Stdout.
type BoundariesConfig struct {
	Files []string
	Out   io.Writer
}

// runCheckBoundaries compiles the given .zen files with zero backends and
// reports every module-boundary violation the resolver already detects during
// a normal Resolve(). This is a report-only view over enforcement that runs on
// every compile (relations, RPC return types and permission resources that
// cross a module boundary are all rejected there), not a new mechanism.
//
// Every diagnostic is printed first, unconditionally, so syntax/resolve errors
// are never hidden behind the boundary-specific section that follows.
func runCheckBoundaries(args []string) error {
	paths, err := resolveInputFiles(args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		reportAutoDiscovery(paths)
	}

	return runCheckBoundariesWith(BoundariesConfig{Files: paths})
}

// runCheckBoundariesWith resolves cfg.Files (one shared resolved schema)
// and reports every cross-module boundary violation to cfg.Out. It returns
// a non-nil error iff at least one violation is found.
func runCheckBoundariesWith(cfg BoundariesConfig) error {
	out := outOrStdout(cfg.Out)

	files, err := loadFiles(cfg.Files)
	if err != nil {
		return err
	}

	schemaDir := defaultSchemaDir
	if pc, err := loadProjectConfig(); err == nil {
		schemaDir = pc.SchemaDir
	}

	result, diags := compile.WithSchemaDir(files, schemaDir)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	// Reuse the same shared boundary check that the resolver uses internally,
	// so this command never drifts from compile-time enforcement.
	boundaryDiags := resolver.CheckCrossModule(result.Schema)
	violations := 0

	for _, d := range boundaryDiags.Sorted() {
		// Proof: CheckCrossModule (internal/dsl/resolver/boundary.go) only
		// ever appends diag.Wrap(..., ErrCrossModule, ...); it never returns
		// other sentinels, so no errors.Is filter is needed and every
		// element here is a violation by construction.
		if violations == 0 {
			_, _ = fmt.Fprintln(out, bold("cross-module boundary violations:"))
		}

		violations++

		_, _ = fmt.Fprintf(out, "  %s  %s\n", red("✗"), dim(d.Pos.String())+" "+d.Msg)
	}

	if violations == 0 {
		_, _ = fmt.Fprintln(out, success("✓ ")+" "+dim("zever check-boundaries: no cross-module violations"))
		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("all modules respect boundaries — nice!"))
		}

		return nil
	}

	return fmt.Errorf("zever check-boundaries: %d cross-module violation(s)", violations)
}
