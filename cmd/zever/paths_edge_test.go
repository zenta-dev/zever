package main

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// TestFlagUsageError_ErrorUnwrap covers the flag-misuse wrapper's message and
// multi-Unwrap contract: errors.Is matches both the sentinel and the
// underlying pflag error.
func TestFlagUsageError_ErrorUnwrap(t *testing.T) {
	t.Parallel()

	inner := errors.New("unknown flag: --nope")
	e := &flagUsageError{err: inner}

	if got, want := e.Error(), errFlagUsage.Error()+": "+inner.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(e, errFlagUsage) {
		t.Fatal("errors.Is(e, errFlagUsage) = false, want true")
	}
	if !errors.Is(e, inner) {
		t.Fatal("errors.Is(e, inner) = false, want true")
	}
}

// TestVersionRequested_flags covers both the --V alias and the cobra-managed
// --version flag, plus the unset default.
func TestVersionRequested_flags(t *testing.T) {
	cmd := newRootCmd()
	if versionRequested(cmd) {
		t.Fatal("versionRequested(no flags) = true, want false")
	}

	alias := newRootCmd()
	if err := alias.ParseFlags([]string{"--V"}); err != nil {
		t.Fatalf("ParseFlags(--V): %v", err)
	}
	if !versionRequested(alias) {
		t.Fatal("versionRequested(--V) = false, want true")
	}

	long := newRootCmd()
	long.InitDefaultVersionFlag()
	if err := long.ParseFlags([]string{"--version"}); err != nil {
		t.Fatalf("ParseFlags(--version): %v", err)
	}
	if !versionRequested(long) {
		t.Fatal("versionRequested(--version) = false, want true")
	}
}

// TestPrintCLIVersion_stdout pins that version output goes to stdout.
func TestPrintCLIVersion_stdout(t *testing.T) {
	out := captureStdout(t, printCLIVersion)
	if !strings.Contains(out, "zever v"+cliVersion) {
		t.Fatalf("stdout = %q, want %q", out, "zever v"+cliVersion)
	}
}

// TestNewRootCmd_VFlagPrintsVersion drives the root RunE version branch: --V
// prints the version and returns nil without a usage dump.
func TestNewRootCmd_VFlagPrintsVersion(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--V"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute(--V): %v", err)
		}
	})
	if !strings.Contains(out, "zever v"+cliVersion) {
		t.Fatalf("stdout = %q, want %q", out, "zever v"+cliVersion)
	}
}

// TestPrintHelpIfRequested_paths covers the no-help and help-token branches
// of the pass-through help guard.
func TestPrintHelpIfRequested_paths(t *testing.T) {
	t.Parallel()

	cmd := newRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := printHelpIfRequested(cmd, []string{"entity"}); err != nil {
		t.Fatalf("printHelpIfRequested(entity) = %v, want nil", err)
	}
	for _, tok := range []string{"-h", "--help", "help"} {
		if err := printHelpIfRequested(cmd, []string{tok}); !errors.Is(err, errHelpShown) {
			t.Fatalf("printHelpIfRequested(%q) = %v, want errHelpShown", tok, err)
		}
	}
}
