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
// builds it from argv. A nil Out defaults to os.Stdout. JSON emits a
// machine-readable envelope instead of the human violation list.
type BoundariesConfig struct {
	Files []string
	Out   io.Writer
	JSON  bool
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
	if err := rejectUnknownFlags("check-boundaries", args, nil); err != nil {
		return err
	}

	paths, err := resolveInputFiles(args)
	if err != nil {
		return fmt.Errorf("zever check-boundaries: %w", err)
	}

	if len(args) == 0 {
		reportAutoDiscovery(paths)
	}

	return runCheckBoundariesWith(BoundariesConfig{Files: paths, JSON: jsonMode})
}

// runCheckBoundariesWith resolves cfg.Files (one shared resolved schema)
// and reports every cross-module boundary violation to cfg.Out. It returns
// a non-nil error iff at least one violation is found.
func runCheckBoundariesWith(cfg BoundariesConfig) error {
	out := outOrStdout(cfg.Out)

	files, err := loadFiles(cfg.Files)
	if err != nil {
		return fmt.Errorf("zever check-boundaries: %w", err)
	}

	pc, err := loadProjectConfig()
	if err != nil {
		return fmt.Errorf("zever check-boundaries: %w", err)
	}

	schemaDir := pc.SchemaDir

	result, diags := compile.WithSchemaDir(files, schemaDir)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	// Reuse the same shared boundary check that the resolver uses internally,
	// so this command never drifts from compile-time enforcement.
	boundaryDiags := resolver.CheckCrossModule(result.Schema)
	violations := 0
	messages := []string{}

	for _, d := range boundaryDiags.Sorted() {
		// Proof: CheckCrossModule (dsl/resolver/boundary.go) only
		// ever appends diag.Wrap(..., ErrCrossModule, ...); it never returns
		// other sentinels, so no errors.Is filter is needed and every
		// element here is a violation by construction.
		if violations == 0 && !cfg.JSON {
			_, _ = fmt.Fprintln(out, bold("cross-module boundary violations:"))
		}

		violations++
		messages = append(messages, d.Pos.String()+" "+d.Msg)

		if !cfg.JSON {
			_, _ = fmt.Fprintf(out, "  %s  %s\n", red("✗"), dim(d.Pos.String())+" "+d.Msg)
		}
	}

	if violations == 0 {
		if cfg.JSON {
			emitSuccess("check-boundaries", map[string]any{"files": cfg.Files, "violations": 0})

			return nil
		}

		_, _ = fmt.Fprintln(out, success("✓ ")+" "+dim("zever check-boundaries: no cross-module violations"))
		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("all modules respect boundaries — nice!"))
		}

		return nil
	}

	err = fmt.Errorf("zever check-boundaries: %d cross-module violation(s)", violations)
	if cfg.JSON {
		return emitFailure("check-boundaries", map[string]any{
			"files":      cfg.Files,
			"violations": violations,
			"messages":   messages,
		}, nil, err)
	}

	return err
}
