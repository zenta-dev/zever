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
	err := runConfig([]string{"show", "--config", "nope.yaml"})
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("error = %q, want it attributed to config loading", err.Error())
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
	cfgPath := writeInspectFixture(t, dir, "zever.yaml", "{}\n")

	var out bytes.Buffer
	if err := runConfigShowWith(ConfigConfig{ConfigPath: cfgPath, Out: &out}); err != nil {
		t.Fatalf("runConfigShowWith with config file: %v", err)
	}
	if !strings.Contains(out.String(), "db") {
		t.Fatalf("output = %q, want the db service row", out.String())
	}
}

func TestRunConfigShowWithCoreBadConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeInspectFixture(t, dir, "zever.yaml", "not: [valid\n")

	var out bytes.Buffer
	if err := runConfigShowWith(ConfigConfig{ConfigPath: cfgPath, Out: &out}); err == nil {
		t.Fatal("expected error for unparsable config file, got nil")
	}
}

func TestPrintConfigUsageColoredBox(t *testing.T) {
	prev := colorEnabled
	colorEnabled = true
	t.Cleanup(func() { colorEnabled = prev })

	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	printConfigUsage(fs)
}
