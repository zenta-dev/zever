package main

import (
	"flag"
	"fmt"
	"strings"
)

// Usage strings for the global flags. root.go registers the Cobra flags
// with these and printFlagDefaults prints them, so the legacy help path and
// `--help` never drift apart.
const (
	usageInteractive = "guided prompts where supported (also ZEVER_INTERACTIVE=1)"
	usageQuiet       = "suppress non-essential output"
	usageNoColor     = "disable ANSI styling"
	usageJSON        = "machine-readable JSON envelope output (also ZEVER_JSON=1)"
	usageV           = "print the CLI version (alias for --version)"
)

// printFlagDefaults writes fs's flags to fs.Output in the same `--name`
// style Cobra uses (single-letter flags keep one dash), followed by the
// global flags, so every legacy usage screen lists --json. It replaces
// flag.FlagSet.PrintDefaults, which prints single-dash names and knows
// nothing about the global flags peeled before the flag set sees argv.
func printFlagDefaults(fs *flag.FlagSet) {
	w := fs.Output()

	fs.VisitAll(func(f *flag.Flag) {
		typeName, usage := flag.UnquoteUsage(f)

		var b strings.Builder

		b.WriteString("  ")

		if len(f.Name) == 1 {
			b.WriteString("-")
		} else {
			b.WriteString("--")
		}

		b.WriteString(f.Name)

		if typeName != "" {
			b.WriteString(" " + typeName)
		}

		b.WriteString("\n        " + strings.ReplaceAll(usage, "\n", "\n        "))

		switch f.DefValue {
		case "", "0", "false", "0s":
		default:
			if typeName == "string" {
				_, _ = fmt.Fprintf(&b, " (default %q)", f.DefValue)
			} else {
				_, _ = fmt.Fprintf(&b, " (default %s)", f.DefValue)
			}
		}

		_, _ = fmt.Fprintln(w, b.String())
	})

	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Global flags:")

	for _, g := range [][2]string{
		{"--json", usageJSON},
		{"-i, --interactive", usageInteractive},
		{"-q, --quiet", usageQuiet},
		{"--no-color", usageNoColor},
		{"-V", usageV},
	} {
		_, _ = fmt.Fprintf(w, "  %-18s %s\n", g[0], g[1])
	}
}
