package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

// captureZeverStderr captures os.Stderr written during fn. It mirrors the
// dirty captureStderr helper under a collision-free name.
func captureZeverStderr(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stderr

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stderr = w

	fn()

	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close pipe writer: %v", closeErr)
	}

	os.Stderr = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	return string(out)
}

// captureZeverStdout captures os.Stdout written during fn.
func captureZeverStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stdout = w

	fn()

	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close pipe writer: %v", closeErr)
	}

	os.Stdout = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	return string(out)
}

// TestRunDBUnknownSubcommand covers the `zever db` dispatcher.
func TestRunDBUnknownSubcommand(t *testing.T) {
	if err := runDB(nil); !errors.Is(err, errDBUnknownSubcommand) {
		t.Fatalf("runDB(nil) error = %v, want errDBUnknownSubcommand", err)
	}

	if err := runDB([]string{"bogus-subcommand"}); err == nil {
		t.Fatalf("expected error for unknown db subcommand, got nil")
	}

	if err := runDB([]string{"--help"}); err != nil {
		t.Fatalf("runDB --help: %v", err)
	}
}

// TestDBUsageListsEverySubcommand keeps the usage text honest as the
// dispatcher grows.
func TestDBUsageListsEverySubcommand(t *testing.T) {
	output := captureZeverStderr(t, printDBUsage)

	for _, want := range []string{"migrate", "rollback", "seed"} {
		if !strings.Contains(output, want) {
			t.Fatalf("db usage missing %q:\n%s", want, output)
		}
	}
}

// TestRunDBInteractiveFlag peels -i and falls through to the missing-command
// error on a headless box (no TTY to prompt on).
func TestRunDBInteractiveFlag(t *testing.T) {
	interactiveMode = false
	t.Cleanup(func() { interactiveMode = false })

	if err := runDB([]string{"-i"}); !errors.Is(err, errDBUnknownSubcommand) {
		t.Fatalf("runDB(-i) error = %v, want errDBUnknownSubcommand", err)
	}
}

// TestRunDBSuggestsClosest pins the did-you-mean hint on typos.
func TestRunDBSuggestsClosest(t *testing.T) {
	output := captureZeverStderr(t, func() {
		_ = runDB([]string{"migrat"})
	})

	if !strings.Contains(output, `"migrate"`) {
		t.Fatalf("expected a did-you-mean hint, got:\n%s", output)
	}
}

// TestCoverRunDBInteractiveDispatch drives the empty-args chooser via seam.
func TestCoverRunDBInteractiveDispatch(t *testing.T) {
	for _, sub := range allDB {
		t.Run(sub, func(t *testing.T) {
			stubPromptTTY(t, true)
			setPromptInteractive(t)
			orig := promptSelectForDB
			promptSelectForDB = func(string, []string) (string, error) { return sub, nil }
			t.Cleanup(func() { promptSelectForDB = orig })
			_ = runDB(nil)
		})
	}
}

// TestCoverRunDBInteractivePromptError covers prompt failure fallthrough.
func TestCoverRunDBInteractivePromptError(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)
	orig := promptSelectForDB
	promptSelectForDB = func(string, []string) (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { promptSelectForDB = orig })
	if err := runDB(nil); !errors.Is(err, errDBUnknownSubcommand) {
		t.Fatalf("runDB prompt err = %v", err)
	}
}

// TestCoverRunDBDispatch covers direct subcommand arms + help.
func TestCoverRunDBDispatch(t *testing.T) {
	prev := interactiveMode
	t.Cleanup(func() { interactiveMode = prev })
	_ = runDB([]string{"--help"})
	_ = runDB([]string{"-h"})
	_ = runDB([]string{"help"})
	_ = runDB([]string{"migrate", "--help"})
	_ = runDB([]string{"rollback", "--help"})
	_ = runDB([]string{"seed", "--help"})
}

// TestUsagePrintersWithColor exercises the styled branches of every usage
// printer in this family by forcing color on and off.
func TestUsagePrintersWithColor(t *testing.T) {
	orig := colorEnabled
	t.Cleanup(func() { colorEnabled = orig })

	for _, on := range []bool{true, false} {
		colorEnabled = on

		_ = captureZeverStderr(t, printDBUsage)
		printDevUsage(io.Discard)
		printLauncherHelp(io.Discard, "zever serve", "desc", "zever serve")
		_ = styledTinkerBanner()

		fs := flag.NewFlagSet("color", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		printMigrateUsage(fs)
		printTinkerUsage(fs)

		_ = captureZeverStdout(t, func() {
			printDryRun(map[string][]byte{"schema.hcl": []byte("hcl")}, []string{`CREATE TABLE "t" (id text)`})
		})
		_ = captureZeverStdout(t, func() {
			printDryRun(nil, nil)
		})
	}
}
