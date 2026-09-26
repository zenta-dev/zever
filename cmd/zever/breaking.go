package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/zenta-dev/zever/dsl/breaking"
	"github.com/zenta-dev/zever/dsl/compile"
)

// expandDirArgs replaces every bare-directory entry in paths with every .zen
// file recursively discovered under it (flat, versioned, or arbitrarily
// nested layouts all resolve the same way as everywhere else in the CLI --
// see walkZenFiles in prompt.go), leaving glob-expanded files and any other
// entry untouched. This lets `zever breaking old-schema -- new-schema` work
// without the caller spelling out `old-schema/*.zen`.
func expandDirArgs(paths []string) ([]string, error) {
	expanded := make([]string, 0, len(paths))

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			// Not a directory (or unreadable) -- leave it for loadFiles to
			// read directly, which reports the real error if any.
			expanded = append(expanded, p)

			continue
		}

		found, walkErr := walkZenFiles(p)
		if walkErr != nil {
			return nil, fmt.Errorf("zever breaking: scan %q: %w", p, walkErr)
		}

		expanded = append(expanded, found...)
	}

	return expanded, nil
}

//nolint:unused
const breakingUsage = `zever breaking <old-files...> -- <new-files...>

Compiles two versions of a schema separately and reports every API-breaking
change between them (removed entity/message/service/rpc/field, a changed
field/param/return type, a changed HTTP method or path). Additions and new
@validate rules are reported too, but as non-breaking, informational
changes. Exits non-zero if any breaking change is found, so it can gate CI.

Each side accepts individual files, a shell-expanded glob, or a bare
directory -- a directory is recursively discovered for every .zen file
under it (flat, versioned, or arbitrarily nested layouts all work), so
` + "`zever breaking schema-old -- schema`" + ` behaves the same as spelling
out ` + "`schema-old/*.zen`" + `.

Flags:`

// BreakingConfig carries every input runBreakingWith needs. Screen agents
// build it from huh forms; the flag shell (runBreaking) splits argv on the
// literal "--" separator. A nil Out defaults to os.Stdout.
type BreakingConfig struct {
	OldFiles []string
	NewFiles []string
	Out      io.Writer
}

// printBreakingUsage prints styled help for `zever breaking`.
func printBreakingUsage(fs *flag.FlagSet) {
	header := title("zever breaking") + dim(" — report API-breaking changes between two schema versions")
	usage := bold("Usage:") + "  " + cmd("zever breaking") + dim("  ") + cyan("<old-files...> -- <new-files...>")
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
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever breaking schema-old/*.zen -- schema/*.zen"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("git show main:schema/app.zen > /tmp/old.zen && zever breaking /tmp/old.zen -- schema/app.zen"))
}

// errNoSeparator is returned when runBreaking gets no "--" separator.
var errNoSeparator = errors.New("zever breaking: expected '<old-files...> -- <new-files...>', no '--' separator found")

// runBreaking compares an "old" and a "new" schema (each its own file set,
// separated by a literal "--" positional argument) and reports every
// breaking.Change between them. It exits non-zero iff breaking.HasBreaking
// finds at least one Breaking change, so it doubles as a CI gate.
func runBreaking(args []string) error {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("breaking", flag.ContinueOnError)
		fs.Usage = func() { printBreakingUsage(fs) }
		// Proof: fs.Usage() calls printBreakingUsage(fs) verbatim, so output
		// is identical to a direct call while also covering the Usage closure.
		fs.Usage()

		return nil
	}

	sepIdx := -1

	for i, a := range args {
		if a == "--" {
			sepIdx = i

			break
		}
	}

	if sepIdx < 0 {
		return errNoSeparator
	}

	return runBreakingWith(BreakingConfig{
		OldFiles: args[:sepIdx],
		NewFiles: args[sepIdx+1:],
	})
}

// runBreakingWith compiles the old and new file sets separately (one shared
// resolved schema each) and reports every breaking.Change between them to
// cfg.Out. It returns a non-nil error iff a breaking change is found.
func runBreakingWith(cfg BreakingConfig) error {
	out := outOrStdout(cfg.Out)

	oldArgs, err := expandDirArgs(cfg.OldFiles)
	if err != nil {
		return err
	}

	newArgs, err := expandDirArgs(cfg.NewFiles)
	if err != nil {
		return err
	}

	oldFiles, err := loadFiles(oldArgs)
	if err != nil {
		return fmt.Errorf("zever breaking: old schema: %w", err)
	}

	newFiles, err := loadFiles(newArgs)
	if err != nil {
		return fmt.Errorf("zever breaking: new schema: %w", err)
	}

	oldResult, oldDiags := compile.Compile(oldFiles)
	if oldDiags.HasErrors() {
		printDiagnostics(oldDiags)

		return fmt.Errorf("zever breaking: old schema (%d file(s)) failed to compile", len(oldFiles))
	}

	newResult, newDiags := compile.Compile(newFiles)
	if newDiags.HasErrors() {
		printDiagnostics(newDiags)

		return fmt.Errorf("zever breaking: new schema (%d file(s)) failed to compile", len(newFiles))
	}

	changes := breaking.Compare(oldResult.Schema, newResult.Schema)

	if len(changes) == 0 {
		_, _ = fmt.Fprintln(out, successMark()+" "+bold("no differences found"))

		return nil
	}

	printBreakingChanges(out, changes)

	if breaking.HasBreaking(changes) {
		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("breaking changes found; bump the API version or revert them"))
		}

		return fmt.Errorf("zever breaking: %d breaking change(s) found", countBreaking(changes))
	}

	_, _ = fmt.Fprintln(out, successMark()+" "+bold("no breaking changes")+dim(fmt.Sprintf(" (%d informational)", len(changes))))

	return nil
}

// printBreakingChanges writes every change to out, breaking changes in
// red/bold via the same severity-coloring convention printDiagnostics uses
// for errors, informational ones dimmed.
func printBreakingChanges(out io.Writer, changes []breaking.Change) {
	for _, c := range changes {
		if c.Breaking {
			_, _ = fmt.Fprintln(out, red(failMark()+" "+c.Message)+dim(" ("+string(c.Kind)+")"))

			continue
		}

		_, _ = fmt.Fprintln(out, dim("  "+c.Message+" ("+string(c.Kind)+")"))
	}
}

func countBreaking(changes []breaking.Change) int {
	n := 0

	for _, c := range changes {
		if c.Breaking {
			n++
		}
	}

	return n
}
