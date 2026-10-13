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
// a bug in this command. The error itself carries the exit-1 contract (see
// TestDoctorHumanFailsOnBatteryFailure); here only the absence of a panic
// matters.
func TestRunDoctorDoesNotPanic(_ *testing.T) {
	_ = runDoctor(nil)
}

func TestRunDoctorInteractiveFlag(t *testing.T) {
	prev := interactiveMode
	t.Cleanup(func() { interactiveMode = prev })

	dir := t.TempDir()
	writeInspectFixture(t, dir, "zever.yaml", doctorTwoBatteryFixture)
	t.Chdir(dir)

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
	dir := t.TempDir()
	cfgPath := writeInspectFixture(t, dir, "zever.yaml", "log:\n  adapter: slog\nrouter:\n  adapter: stdhttp\ndb:\n  adapter: sqlite\npassword:\n  adapter: argon2id\nqueue:\n  adapter: memory\nworkflow:\n  adapter: memory\n")

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{ConfigPath: cfgPath, Out: &out}); err != nil {
		t.Fatalf("runDoctorWith: %v", err)
	}

	got := out.String()
	for _, want := range []string{"config", "db", "password", "queue", "workflow"} {
		if _, ok := doctorLine(got, want); !ok {
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

// TestRunDoctorWithAppliesEnvOverrides proves `zever doctor` with no
// --config resolves through config.Load(""), so env overrides apply before
// batteries are resolved -- the same config the container would build.
func TestRunDoctorWithAppliesEnvOverrides(t *testing.T) {
	t.Setenv("DB_ADAPTER", "postgres")

	prev := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prev })

	var gotAdapter string

	doctorChecksFor = func(_ *container.Container, resolved *config.Config) map[string]func() error {
		gotAdapter = resolved.DB.Adapter

		return map[string]func() error{"config": func() error { return nil }}
	}

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err != nil {
		t.Fatalf("runDoctorWith: %v", err)
	}

	if gotAdapter != "postgres" {
		t.Fatalf("resolved DB adapter = %q, want postgres from DB_ADAPTER env", gotAdapter)
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

	dir := t.TempDir()
	writeInspectFixture(t, dir, "zever.yaml", doctorTwoBatteryFixture)
	t.Chdir(dir)

	if err := runDoctor([]string{"--interactive"}); err != nil {
		t.Fatalf("runDoctor --interactive: %v", err)
	}
	if !interactiveMode {
		t.Fatalf("expected --interactive to set interactiveMode")
	}
}

// TestRunDoctorStrictFailsOnBatteryFailure covers --strict: a failing
// battery surfaces a non-nil error so `zever doctor --strict` (and plain
// `zever doctor`, since P3-3) gates a CI/pre-deploy pipeline on exit code.
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

// TestRunDoctorHumanFailsWithoutStrict pins the P3-3 exit-code fix: a
// failing battery fails the command even without --strict (exit 1).
func TestRunDoctorHumanFailsWithoutStrict(t *testing.T) {
	prev := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prev })
	doctorChecksFor = func(_ *container.Container, _ *config.Config) map[string]func() error {
		return map[string]func() error{
			"config": func() error { return nil },
			"db":     func() error { return errFakeBatteryFailure },
		}
	}

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err == nil {
		t.Fatal("runDoctorWith Strict=false with a failing battery: got nil error, want non-nil for exit 1")
	}
}

var errFakeBatteryFailure = errors.New("fake battery failure")

func TestRunDoctorFlagsDevKeyInNonDevEnv(t *testing.T) {
	t.Setenv("ZEVER_ENV", "")

	var out bytes.Buffer
	// Defaults fail other batteries too (auth/jwt has no secret), so the
	// command errors; the assertion is the crypto row flagging the dev key.
	if err := runDoctorWith(DoctorConfig{Out: &out}); err == nil {
		t.Fatal("runDoctorWith defaults: got nil error, want non-nil")
	}

	line, ok := doctorLine(out.String(), "crypto")
	if !ok {
		t.Fatalf("output missing crypto row:\n%s", out.String())
	}
	if !strings.Contains(line, "dev key") {
		t.Errorf("crypto row %q does not flag the dev key", line)
	}
}

func TestRunDoctorDevKeyAllowedInDevEnv(t *testing.T) {
	t.Setenv("ZEVER_ENV", "test")

	var out bytes.Buffer
	// Other default batteries still fail (auth/jwt has no secret), so the
	// command errors; the assertion is the crypto row passing in dev env.
	if err := runDoctorWith(DoctorConfig{Out: &out}); err == nil {
		t.Fatal("runDoctorWith defaults: got nil error, want non-nil")
	}

	line, ok := doctorLine(out.String(), "crypto")
	if !ok {
		t.Fatalf("output missing crypto row:\n%s", out.String())
	}
	if strings.Contains(line, "FAIL") {
		t.Errorf("crypto row %q not OK in dev env", line)
	}
}

// doctorLine returns the output line whose first field is the battery name.
func doctorLine(out, name string) (string, bool) {
	for line := range strings.Lines(out) {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return line, true
		}
	}
	return "", false
}

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
