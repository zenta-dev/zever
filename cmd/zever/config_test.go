package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
)

func TestRunConfigMissingSubcommand(t *testing.T) {
	err := runConfig(nil)
	if err == nil {
		t.Fatal("expected errConfigUnknownSubcommand, got nil")
	}
	if !strings.Contains(err.Error(), "missing subcommand") {
		t.Fatalf("error = %q, want missing subcommand", err.Error())
	}
}

func TestRunConfigUnknownSubcommand(t *testing.T) {
	err := runConfig([]string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand, got nil")
	}
	if !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("error = %q, want unknown subcommand", err.Error())
	}
}

func TestRunConfigHelpVariants(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		if err := runConfig(args); err != nil {
			t.Fatalf("runConfig(%v): %v", args, err)
		}
	}
}

func TestRunConfigShowDoesNotPanic(t *testing.T) {
	if err := runConfig([]string{"show"}); err != nil {
		t.Fatalf("runConfig show: %v", err)
	}
}

func TestRunConfigShowHelpPrintsUsage(t *testing.T) {
	if err := runConfig([]string{"show", "-h"}); err != nil {
		t.Fatalf("runConfig show -h: %v", err)
	}
}

func TestRunConfigShowFlagParseError(t *testing.T) {
	if err := runConfig([]string{"show", "--bogus-flag"}); err == nil {
		t.Fatal("expected flag parse error, got nil")
	}
}

func TestRunConfigShowMissingConfigFile(t *testing.T) {
	// There is no --config file flag: service config is env-only, so
	// naming it is an unknown-flag rejection, never a load error.
	err := runConfig([]string{"show", "--config", "nope.yaml"})
	if err == nil {
		t.Fatal("expected error for removed --config flag, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("error = %q, want it to report an unknown flag", err.Error())
	}
}

func TestRunConfigShowWithCoreListsServices(t *testing.T) {
	var out bytes.Buffer
	if err := runConfigShowWith(ConfigConfig{Out: &out}); err != nil {
		t.Fatalf("runConfigShowWith: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ai", "db", "cache", "queue", "workflow", "adapter="} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRunConfigShowWithCoreRedactsSecrets(t *testing.T) {
	var out bytes.Buffer
	if err := runConfigShowWith(ConfigConfig{Out: &out}); err != nil {
		t.Fatalf("runConfigShowWith: %v", err)
	}

	got := out.String()
	if strings.Contains(got, config.Default().Crypto.Options.Key) {
		t.Fatalf("output leaks crypto key:\n%s", got)
	}
	if !strings.Contains(got, config.RedactedValue) {
		t.Fatalf("output missing redaction marker:\n%s", got)
	}
}

func TestRunConfigShowWithCoreConfigFile(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", "LOG_ADAPTER=slog\n")
	t.Chdir(dir)

	var out bytes.Buffer
	if err := runConfigShowWith(ConfigConfig{Out: &out}); err != nil {
		t.Fatalf("runConfigShowWith with env file: %v", err)
	}
	if !strings.Contains(out.String(), "db") {
		t.Fatalf("output = %q, want the db service row", out.String())
	}
}

func TestRunConfigShowWithCoreBadConfigFile(t *testing.T) {
	// A malformed .env in the working directory fails resolution closed.
	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", "this line has no equals sign\n")
	t.Chdir(dir)

	var out bytes.Buffer
	if err := runConfigShowWith(ConfigConfig{Out: &out}); err == nil {
		t.Fatal("expected error for unparsable .env, got nil")
	}
}

// TestRunConfigShowWithAppliesEnvOverrides proves `zever config show` with no
// --config resolves through dotenv discovery plus the process environment,
// so env overrides apply exactly as they do when the container builds its
// config.
func TestRunConfigShowWithAppliesEnvOverrides(t *testing.T) {
	t.Setenv("DB_ADAPTER", "postgres")

	var out bytes.Buffer
	if err := runConfigShowWith(ConfigConfig{Out: &out}); err != nil {
		t.Fatalf("runConfigShowWith: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "db  adapter=postgres") {
		t.Fatalf("output = %q, want db adapter=postgres from DB_ADAPTER env", got)
	}
}

func TestPrintConfigUsageColoredBox(t *testing.T) {
	prev := colorEnabled
	colorEnabled = true
	t.Cleanup(func() { colorEnabled = prev })

	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	printConfigUsage(fs)
}

// TestRunConfigShowPrintsNamedInstances proves per-instance output:
// SERVICE__NAME__* env renders a redacted "service/NAME" section beside the
// default row, and secret values never appear raw.
func TestRunConfigShowPrintsNamedInstances(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", "DB_ADAPTER=sqlite\nDB__ANALYTICS__ADAPTER=postgres\nDB__ANALYTICS__DSN=postgres://user:s3cret@localhost/db?sslmode=disable\n")
	t.Chdir(dir)

	var out bytes.Buffer
	if err := runConfigShowWith(ConfigConfig{Out: &out}); err != nil {
		t.Fatalf("runConfigShowWith: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "db/ANALYTICS") {
		t.Fatalf("output missing db/ANALYTICS instance section:\n%s", got)
	}

	if strings.Contains(got, "s3cret") {
		t.Fatalf("output leaks instance secret:\n%s", got)
	}
}

// TestRunConfigShowTypoEnvHints pins the typo'd-env-var surface: an unknown
// key under a wired service prefix fails with a did-you-mean hint naming
// the exact key, so the operator knows what to fix and where.
func TestRunConfigShowTypoEnvHints(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DB_MAXCONN", "5")

	var out bytes.Buffer
	err := runConfigShowWith(ConfigConfig{Out: &out})
	if err == nil {
		t.Fatal("runConfigShowWith typo env: got nil error, want non-nil")
	}

	if !strings.Contains(err.Error(), "DB_MAXCONN") {
		t.Fatalf("error = %q, want it to name the exact env key", err.Error())
	}

	if !strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("error = %q, want a did-you-mean hint", err.Error())
	}

	if !strings.Contains(err.Error(), "DB_MAX_CONNS") {
		t.Fatalf("error = %q, want the corrected env key", err.Error())
	}
}
