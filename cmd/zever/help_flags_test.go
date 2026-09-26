package main

import (
	"strings"
	"testing"
)

// TestCobraNewHelpExposesFlags pins W3: `zever new -h/--help` must list the
// real stdlib flags, not just cobra globals.
func TestCobraNewHelpExposesFlags(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		t.Run("new "+flag, func(t *testing.T) {
			stdout, stderr, err := executeRoot(t, "new", flag)
			if err != nil {
				t.Fatalf("new %s: expected exit 0, got %v (stderr: %s)", flag, err, stderr)
			}

			combined := stdout + stderr
			if !strings.Contains(combined, "Usage:") {
				t.Errorf("new %s: output missing Usage: (got %q)", flag, combined)
			}

			for _, want := range []string{"--module", "--dir", "--framework-version", "--force"} {
				if !strings.Contains(combined, want) {
					t.Errorf("new %s: output missing %q (got %q)", flag, want, combined)
				}
			}
		})
	}
}

// TestCobraCompileHelpExposesFlags pins W3: `zever compile -h/--help` must
// list --backend and --out with the ./generated default.
func TestCobraCompileHelpExposesFlags(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		t.Run("compile "+flag, func(t *testing.T) {
			stdout, stderr, err := executeRoot(t, "compile", flag)
			if err != nil {
				t.Fatalf("compile %s: expected exit 0, got %v (stderr: %s)", flag, err, stderr)
			}

			combined := stdout + stderr
			if !strings.Contains(combined, "Usage:") {
				t.Errorf("compile %s: output missing Usage: (got %q)", flag, combined)
			}

			for _, want := range []string{"--backend", "--out", "./generated"} {
				if !strings.Contains(combined, want) {
					t.Errorf("compile %s: output missing %q (got %q)", flag, want, combined)
				}
			}
		})
	}
}

// TestCobraHelpPositionalHelp pins uniform `help` positional handling.
func TestCobraHelpPositionalHelp(t *testing.T) {
	for _, args := range [][]string{{"new", "help"}, {"compile", "help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, err := executeRoot(t, args...)
			if err != nil {
				t.Fatalf("%v: expected exit 0, got %v", args, err)
			}
		})
	}
}

// TestLegacyHelpNewExposesFlags pins `zever help new` (run dispatch) to the
// same real flags.
func TestLegacyHelpNewExposesFlags(t *testing.T) {
	resetInteractiveMode(t)

	out := captureStderr(t, func() {
		if err := run([]string{"help", "new"}); err != nil {
			t.Fatalf("run help new = %v, want nil", err)
		}
	})
	_ = t.Context()

	for _, want := range []string{"--module", "--dir", "--framework-version", "--force"} {
		if !strings.Contains(out, want) {
			t.Errorf("help new: stderr missing %q (got %q)", want, out)
		}
	}
}

// TestLegacyHelpCompileExposesFlags pins `zever help compile` to the real
// backend/out flags.
func TestLegacyHelpCompileExposesFlags(t *testing.T) {
	resetInteractiveMode(t)

	out := captureStderr(t, func() {
		if err := run([]string{"help", "compile"}); err != nil {
			t.Fatalf("run help compile = %v, want nil", err)
		}
	})
	_ = t.Context()

	for _, want := range []string{"--backend", "--out"} {
		if !strings.Contains(out, want) {
			t.Errorf("help compile: stderr missing %q (got %q)", want, out)
		}
	}
}
