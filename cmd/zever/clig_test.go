package main

import (
	"bytes"
	"strings"
	"testing"
)

// executeRoot runs a FRESH root command with args, capturing output.
// A fresh tree per call (not the shared rootCmd global) keeps tests free of
// cross-test flag-state mutation; Cobra/pflag command state is not safe for
// concurrent Execute, so these tests stay sequential (no t.Parallel).
func executeRoot(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	root := newRootCmd()
	root.SetContext(t.Context())

	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)

	err = root.Execute()

	return outBuf.String(), errBuf.String(), err
}

// cligCommands is the full top-level command surface the compliance tests
// pin. Keep in sync with portedCommands (root.go) as commands migrate.
var cligCommands = []string{
	"new", "compile", "doctor", "config", "routes", "check",
	"breaking", "fmt", "explain", "check-boundaries", "graph",
	"generate", "extract", "serve", "dev", "queue:work",
	"schedule:run", "tinker", "db", "completion", "docs",
}

func TestCligCommandDocs(t *testing.T) {
	root := newRootCmd()

	for _, name := range cligCommands {
		t.Run(name, func(t *testing.T) {
			found, _, err := root.Find([]string{name})
			if err != nil {
				t.Fatalf("Find(%q) error: %v", name, err)
			}
			if found == nil || (found.Name() != name && found.Name() != strings.Split(name, ":")[0]) {
				// Fall back to strict lookup: command must exist under this use name.
				t.Fatalf("command %q not found (got %v)", name, found)
			}

			if found.Short == "" {
				t.Errorf("command %q: Short is empty", name)
			} else if c := found.Short[0]; c < 'A' || c > 'Z' {
				t.Errorf("command %q: Short must start with uppercase verb, got %q", name, found.Short)
			}

			long := strings.TrimSpace(found.Long)
			if long == "" {
				t.Errorf("command %q: Long is empty", name)
			} else if c := long[0]; c < 'A' || c > 'Z' {
				t.Errorf("command %q: Long must start with uppercase verb, got %q", name, long)
			}

			if strings.TrimSpace(found.Example) == "" {
				t.Errorf("command %q: Example is empty", name)
			}
		})
	}
}

func TestCligHelpUsage(t *testing.T) {
	for _, name := range cligCommands {
		for _, flag := range []string{"-h", "--help"} {
			t.Run(name+"/"+flag, func(t *testing.T) {
				stdout, stderr, err := executeRoot(t, name, flag)
				if err != nil {
					t.Fatalf("%s %s: expected exit 0, got error: %v (stderr: %s)", name, flag, err, stderr)
				}
				if combined := stdout + stderr; !strings.Contains(combined, "Usage:") {
					t.Errorf("%s %s: output missing \"Usage:\" (stdout=%q stderr=%q)", name, flag, stdout, stderr)
				}
			})
		}
	}
}

func TestCligHelpRegression(t *testing.T) {
	tests := []string{"routes", "graph", "check-boundaries", "explain"}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, err := executeRoot(t, name, "-h")
			if err != nil {
				t.Fatalf("%s -h: expected exit 0, got error: %v", name, err)
			}
			combined := stdout + stderr
			for _, banned := range []string{"no such file", "no input"} {
				if strings.Contains(combined, banned) {
					t.Errorf("%s -h: output must not contain %q (got %q)", name, banned, combined)
				}
			}
		})
	}
}

func TestCligUnknownCommand(t *testing.T) {
	// closest("compil") must resolve to "compile" via the shared helper.
	if got := closest("compil", allTopLevel); got != "compile" {
		t.Errorf("closest(%q) = %q, want %q", "compil", got, "compile")
	}

	tests := []struct {
		name       string
		args       []string
		wantErrSub string
		wantStderr string
	}{
		{name: "frobnicate", args: []string{"frobnicate"}, wantErrSub: "unknown subcommand", wantStderr: ""},
		{name: "compil", args: []string{"compil"}, wantErrSub: "unknown subcommand", wantStderr: "compile"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Unknown subcommands fall through to the legacy run()
			// dispatch, which prints usage plus the did-you-mean hint
			// to stderr and returns the unknown-subcommand error.
			var runErr error
			stderr := captureStderr(t, func() { runErr = run(tc.args) })
			if runErr == nil {
				t.Fatalf("%v: expected error, got nil", tc.args)
			}
			if !strings.Contains(runErr.Error(), tc.wantErrSub) {
				t.Errorf("%v: error %q missing %q", tc.args, runErr.Error(), tc.wantErrSub)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("%v: stderr missing suggestion %q (stderr=%q)", tc.args, tc.wantStderr, stderr)
			}
		})
	}
}
