package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestPrintFlagDefaults_DoubleDashAndGlobals(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("x", flag.ContinueOnError)

	var buf bytes.Buffer
	fs.SetOutput(&buf)
	fs.String("config", "", "config path")
	fs.Bool("strict", false, "be strict")
	fs.String("out", "./gen", "output dir")
	fs.Bool("w", false, "write")

	printFlagDefaults(fs)

	out := buf.String()
	for _, want := range []string{"--config string", "--strict", "--out string", `(default "./gen")`, "  -w\n", "--json"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	for _, bad := range []string{"\n  -config", "\n  -strict", "\n  -out"} {
		if strings.Contains(out, bad) {
			t.Errorf("output has single-dash flag %q:\n%s", bad, out)
		}
	}
}

// TestLegacyHelpListsJSONAndRealFlags pins every legacy `zever help <cmd>`
// screen to list the global --json flag, and doctor to list its real flags in
// `--` style.
func TestLegacyHelpListsJSONAndRealFlags(t *testing.T) {
	for _, sub := range []string{"doctor", "check", "fmt", "routes", "explain", "graph", "breaking", "config", "extract", "check-boundaries"} {
		resetInteractiveMode(t)

		out := captureStderr(t, func() {
			if err := run([]string{"help", sub}); err != nil {
				t.Fatalf("help %s: %v", sub, err)
			}
		})

		if !strings.Contains(out, "--json") {
			t.Errorf("help %s does not list --json:\n%s", sub, out)
		}
	}

	resetInteractiveMode(t)

	out := captureStderr(t, func() {
		if err := run([]string{"help", "doctor"}); err != nil {
			t.Fatalf("help doctor: %v", err)
		}
	})

	for _, want := range []string{"--config string", "--strict"} {
		if !strings.Contains(out, want) {
			t.Errorf("help doctor missing %q:\n%s", want, out)
		}
	}
}

func TestCobraHelpListsDocumentedFlags(t *testing.T) {
	// Not parallel: Cobra's PersistentPreRun writes package-level mode vars.
	for sub, wants := range map[string][]string{
		"doctor": {"--config", "--strict", "--json"},
		"fmt":    {"--write", "--list", "--json"},
		"new":    {"--adapters", "--profile", "--json"},
	} {
		out, _, err := executeRoot(t, sub, "--help")
		if err != nil {
			t.Fatalf("%s --help: %v", sub, err)
		}

		for _, w := range wants {
			if !strings.Contains(out, w) {
				t.Errorf("%s --help missing %q:\n%s", sub, w, out)
			}
		}
	}
}
