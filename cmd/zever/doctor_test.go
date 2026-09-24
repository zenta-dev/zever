package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

// TestRunDoctorDoesNotPanic asserts doctor runs cleanly against
// config.Default(): some batteries (e.g. auth/jwt with no configured
// secret) are expected to legitimately FAIL, which is correct behavior, not
// a bug in this command.
func TestRunDoctorDoesNotPanic(t *testing.T) {
	if err := runDoctor(nil); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}
}

func TestRunDoctorInteractiveFlag(t *testing.T) {
	prev := interactiveMode
	t.Cleanup(func() { interactiveMode = prev })

	if err := runDoctor([]string{"-i"}); err != nil {
		t.Fatalf("runDoctor -i: %v", err)
	}

	if !interactiveMode {
		t.Fatalf("expected -i to set interactiveMode")
	}
}

func TestRunDoctorHelpPrintsUsage(t *testing.T) {
	if err := runDoctor([]string{"-h"}); err != nil {
		t.Fatalf("runDoctor -h: %v", err)
	}
}

func TestRunDoctorFlagParseError(t *testing.T) {
	if err := runDoctor([]string{"--bogus-flag"}); err == nil {
		t.Fatalf("expected flag parse error, got nil")
	}
}

func TestRunDoctorMissingConfigFile(t *testing.T) {
	err := runDoctor([]string{"--config", "nope.yaml"})
	if err == nil {
		t.Fatalf("expected error for missing config file, got nil")
	}

	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("error = %q, want it attributed to config loading", err.Error())
	}
}

func TestRunDoctorWithCoreListsBatteries(t *testing.T) {
	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err != nil {
		t.Fatalf("runDoctorWith: %v", err)
	}

	got := out.String()
	for _, want := range []string{"config", "ai", "db", "password", "queue", "workflow"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing battery %q:\n%s", want, got)
		}
	}

	// Dropped batteries (no container accessor in zever) must not appear.
	for _, dropped := range []string{"lock", "realtime", "secrets"} {
		for line := range strings.Lines(got) {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == dropped {
				t.Fatalf("output lists dropped battery %q:\n%s", dropped, got)
			}
		}
	}
}

func TestRunDoctorWithCoreConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeInspectFixture(t, dir, "zever.yaml", "{}\n")

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{ConfigPath: cfgPath, Out: &out}); err != nil {
		t.Fatalf("runDoctorWith with config file: %v", err)
	}

	if !strings.Contains(out.String(), "config") {
		t.Fatalf("output = %q, want the config validation row", out.String())
	}
}

func TestRunDoctorWithCoreBadConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeInspectFixture(t, dir, "zever.yaml", "not: [valid\n")

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{ConfigPath: cfgPath, Out: &out}); err == nil {
		t.Fatalf("expected error for unparsable config file, got nil")
	}
}

func TestPrintDoctorUsageColoredBox(t *testing.T) {
	prev := colorEnabled
	colorEnabled = true
	t.Cleanup(func() { colorEnabled = prev })

	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	printDoctorUsage(fs)
}

func TestRunDoctorInteractiveLongForm(t *testing.T) {
	prev := interactiveMode
	t.Cleanup(func() { interactiveMode = prev })

	if err := runDoctor([]string{"--interactive"}); err != nil {
		t.Fatalf("runDoctor --interactive: %v", err)
	}
	if !interactiveMode {
		t.Fatalf("expected --interactive to set interactiveMode")
	}
}

// TestRunDoctorStrictFailsOnBatteryFailure covers --strict: unlike the
// default (always nil -- a FAIL row is an expected outcome, see
// TestRunDoctorDoesNotPanic), --strict must surface a non-nil error so
// `zever doctor --strict` can gate a CI/pre-deploy pipeline on exit code.
func TestRunDoctorStrictFailsOnBatteryFailure(t *testing.T) {
	prev := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prev })
	doctorChecksFor = func(_ *container.Container, _ *config.Config) map[string]func() error {
		return map[string]func() error{
			"config": func() error { return nil },
			"db":     func() error { return errFakeBatteryFailure },
		}
	}

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out, Strict: true}); err == nil {
		t.Fatal("runDoctorWith Strict=true with a failing battery: got nil error, want non-nil")
	}
}

// TestRunDoctorNotStrictIgnoresBatteryFailure pins the default: a failing
// battery never fails the command unless --strict is passed.
func TestRunDoctorNotStrictIgnoresBatteryFailure(t *testing.T) {
	prev := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prev })
	doctorChecksFor = func(_ *container.Container, _ *config.Config) map[string]func() error {
		return map[string]func() error{
			"config": func() error { return nil },
			"db":     func() error { return errFakeBatteryFailure },
		}
	}

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err != nil {
		t.Fatalf("runDoctorWith Strict=false with a failing battery: %v, want nil", err)
	}
}

var errFakeBatteryFailure = errors.New("fake battery failure")

func TestRunDoctorWithAllOK(t *testing.T) {
	prev := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prev })
	doctorChecksFor = func(_ *container.Container, _ *config.Config) map[string]func() error {
		return map[string]func() error{
			"config": func() error { return nil },
			"db":     func() error { return nil },
		}
	}
	t.Setenv("ZEVER_NO_HINT", "")

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err != nil {
		t.Fatalf("runDoctorWith all-OK: %v", err)
	}
	if !strings.Contains(out.String(), "All 2 batteries OK") {
		t.Fatalf("output = %q, want all-OK summary", out.String())
	}
}
