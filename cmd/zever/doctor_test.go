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

	prevChecks := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prevChecks })
	doctorChecksFor = func(_ *container.Container, _ *config.Config) map[string]func() error {
		return map[string]func() error{"config": func() error { return nil }}
	}

	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", doctorTwoBatteryFixture)
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
	// There is no --config file flag: service config is env-only (.env
	// discovery + process env), so naming it is flag misuse (exit 2).
	err := runDoctor([]string{"--config", "nope.yaml"})
	if err == nil {
		t.Fatalf("expected error for removed --config flag, got nil")
	}

	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("error = %q, want it to report an unknown flag", err.Error())
	}
}

func TestRunDoctorWithCoreListsBatteries(t *testing.T) {
	prev := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prev })
	doctorChecksFor = func(_ *container.Container, _ *config.Config) map[string]func() error {
		return map[string]func() error{
			"config":   func() error { return nil },
			"db":       func() error { return nil },
			"password": func() error { return nil },
			"queue":    func() error { return nil },
			"workflow": func() error { return nil },
		}
	}

	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", "LOG_MIN_LEVEL=1\nDB_PATH=data/test.db\nPASSWORD_TIME=1\nQUEUE_BUFFER=8\nWORKFLOW_TABLE=wf\n")
	t.Chdir(dir)

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err != nil {
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
	prev := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prev })
	doctorChecksFor = func(_ *container.Container, _ *config.Config) map[string]func() error {
		return map[string]func() error{"config": func() error { return nil }}
	}

	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", "LOG_MIN_LEVEL=1\n")
	t.Chdir(dir)

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err != nil {
		t.Fatalf("runDoctorWith with env file: %v", err)
	}

	if !strings.Contains(out.String(), "config") {
		t.Fatalf("output = %q, want the config validation row", out.String())
	}
}

func TestRunDoctorWithCoreBadConfigFile(t *testing.T) {
	// A malformed .env in the working directory fails resolution closed:
	// config is env-only, so there is no file flag to point at.
	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", "this line has no equals sign\n")
	t.Chdir(dir)

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err == nil {
		t.Fatalf("expected error for unparsable .env, got nil")
	}
}

// TestRunDoctorWithAppliesEnvOverrides proves `zever doctor` with no
// --config resolves through dotenv discovery plus the process environment,
// so env overrides apply before batteries are resolved -- the same config
// the container would build.
func TestRunDoctorWithAppliesEnvOverrides(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "42")

	// The CLI's own package directory holds a cmd/server fixture, which
	// would make the doctor check the project inside the built server
	// instead of the stubbed local table. Run in an empty directory.
	t.Chdir(t.TempDir())

	prev := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prev })

	var gotMaxConns int

	doctorChecksFor = func(_ *container.Container, resolved *config.Config) map[string]func() error {
		gotMaxConns = resolved.DB.Options.MaxConns

		return map[string]func() error{"config": func() error { return nil }}
	}

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err != nil {
		t.Fatalf("runDoctorWith: %v", err)
	}

	if gotMaxConns != 42 {
		t.Fatalf("resolved DB max_conns = %d, want 42 from DB_MAX_CONNS env", gotMaxConns)
	}
}

// TestRunDoctorRejectsAdapterEnv pins the removal: a legacy *_ADAPTER key
// fails resolution instead of selecting anything.
func TestRunDoctorRejectsAdapterEnv(t *testing.T) {
	t.Setenv("DB_ADAPTER", "postgres")
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err == nil {
		t.Fatal("runDoctorWith with DB_ADAPTER set = nil error, want removal failure")
	} else if !strings.Contains(err.Error(), "adapter selection removed") {
		t.Fatalf("runDoctorWith error = %q, want removal message", err)
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

	prevChecks := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prevChecks })
	doctorChecksFor = func(_ *container.Container, _ *config.Config) map[string]func() error {
		return map[string]func() error{"config": func() error { return nil }}
	}

	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", doctorTwoBatteryFixture)
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
	// Empty directory: no built server entrypoint, so the checks run in
	// the CLI against the stubbed local table.
	t.Chdir(t.TempDir())

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
	// Empty directory: no built server entrypoint, so the checks run in
	// the CLI against the stubbed local table.
	t.Chdir(t.TempDir())

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

	// Empty directory: no built server entrypoint, so every battery is
	// checked in the CLI (the crypto row included).
	t.Chdir(t.TempDir())

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

	// Empty directory: no built server entrypoint, so every battery is
	// checked in the CLI (the crypto row included).
	t.Chdir(t.TempDir())

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
	// Empty directory: no built server entrypoint, so the checks run in
	// the CLI against the stubbed local table.
	t.Chdir(t.TempDir())

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
