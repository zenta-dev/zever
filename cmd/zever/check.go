package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/zenta-dev/zever/internal/dsl/compile"
)

// CheckConfig carries every input runCheckWith needs. Screen agents build it
// from huh forms; the flag shell (runCheck) builds it from argv. A nil Out
// defaults to os.Stdout.
type CheckConfig struct {
	Files []string
	Out   io.Writer
}

// printCheckUsage prints styled help for `zever check`.
func printCheckUsage(fs *flag.FlagSet) {
	header := title("zever check") + dim(" — validate schemas, no output written")
	usage := bold("Usage:") + "  " + cmd("zever check") + dim("  ") + cyan("<files...>")
	body := joinLines(
		header,
		"",
		usage,
		"",
		bold("Flags:"),
	)
	_, _ = fmt.Fprintln(fs.Output(), body)
	fs.PrintDefaults()
	_, _ = fmt.Fprintln(fs.Output(), "")
	_, _ = fmt.Fprintln(fs.Output(), dim("Examples:"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever check schema/app.zen"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever check schema/*.zen"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(fs.Output(), formatHint("run 'zever compile' once check passes"))
	}
}

// runCheck compiles the given .zen files with zero backends (schema
// resolution only, no codegen, no filesystem writes) and reports whether
// the schema is valid. It mirrors runRoutes's no-backend compile.Compile
// call exactly, but only cares about diagnostics, not the resolved schema.
func runCheck(args []string) error {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("check", flag.ContinueOnError)
		fs.Usage = func() { printCheckUsage(fs) }
		// Proof: fs.Usage() calls printCheckUsage(fs) verbatim, so output is
		// identical to a direct call while also covering the Usage closure.
		fs.Usage()

		return nil
	}

	paths, err := resolveInputFiles(args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		reportAutoDiscovery(paths)
	}

	return runCheckWith(CheckConfig{Files: paths})
}

// runCheckWith validates cfg.Files with zero backends and reports validity
// to cfg.Out. It writes nothing to the filesystem.
func runCheckWith(cfg CheckConfig) error {
	out := outOrStdout(cfg.Out)

	files, err := loadFiles(cfg.Files)
	if err != nil {
		return err
	}

	_, diags := compile.Compile(files)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	if diags.HasErrors() {
		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("fix diagnostics above, then re-run check"))
		}

		return fmt.Errorf("zever check: %d file(s) failed to compile", len(files))
	}

	_, _ = fmt.Fprintln(out, successMark()+" "+bold(fmt.Sprintf("%d file(s) valid", len(files))))

	if shouldShowHint() {
		if h := hintFor("check"); h != "" {
			_, _ = fmt.Fprintln(os.Stderr, formatHint(h))
		}
	}

	return nil
}
