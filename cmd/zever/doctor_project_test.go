package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// doctorTwoBatteryFixture enables exactly two zero-infra batteries. Both
// resolve without secrets or network, so the run is green and deterministic.
const doctorTwoBatteryFixture = "log:\n  adapter: slog\nrouter:\n  adapter: stdhttp\n"

// doctorFailingBatteryFixture enables one battery that cannot resolve without
// a configured secret (jwt requires a 32+ byte secret, adapter-validated at
// Open, never at Validate, so config.Load succeeds and the row FAILs).
const doctorFailingBatteryFixture = "auth:\n  adapter: jwt\n"

// doctorTypoBatteryFixture enables one battery with a misspelled adapter:
// ParseAdapter stays permissive so config validation passes, but the
// adapters row fails fast with a did-you-mean hint (P4-1).
const doctorTypoBatteryFixture = "cache:\n  adapter: memmory\n"

// doctorEnabledRows returns the battery names in doctor's human output by
// first-field match, so "ai" never collides with prose like "batteries".
func doctorEnabledRows(t *testing.T, out string) []string {
	t.Helper()

	var names []string

	for line := range strings.Lines(out) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		if _, ok := doctorChecksFor(nil, nil)[fields[0]]; ok {
			names = append(names, fields[0])
		}
	}

	return names
}

// TestDoctorOnlyChecksEnabledBatteries is the P3-3 core contract: a fixture
// project enabling two services gets exactly those rows (plus the config
// validation and adapters rows), never the other 30+ batteries.
func TestDoctorOnlyChecksEnabledBatteries(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeInspectFixture(t, dir, "zever.yaml", doctorTwoBatteryFixture)

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{ConfigPath: cfgPath, Out: &out}); err != nil {
		t.Fatalf("runDoctorWith two-battery fixture: %v", err)
	}

	names := doctorEnabledRows(t, out.String())

	want := map[string]bool{"config": true, "adapters": true, "log": true, "router": true}
	if len(names) != len(want) {
		t.Fatalf("checked batteries = %v, want exactly %v\noutput:\n%s", names, want, out.String())
	}

	for _, n := range names {
		if !want[n] {
			t.Fatalf("checked batteries = %v, want exactly %v\noutput:\n%s", names, want, out.String())
		}
	}
}

// TestDoctorHumanFailsOnBatteryFailure pins the P3-3 exit-code fix: the human
// path returns a non-nil error (exit 1) when any enabled battery fails, with
// no --strict flag needed.
func TestDoctorHumanFailsOnBatteryFailure(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeInspectFixture(t, dir, "zever.yaml", doctorFailingBatteryFixture)

	var out bytes.Buffer
	err := runDoctorWith(DoctorConfig{ConfigPath: cfgPath, Out: &out})
	if err == nil {
		t.Fatal("runDoctorWith failing battery: got nil error, want non-nil for exit 1")
	}

	if exitCodeFor(err) != ExitError {
		t.Fatalf("exitCodeFor(err) = %d, want %d", exitCodeFor(err), ExitError)
	}

	line, ok := doctorLine(out.String(), "auth")
	if !ok {
		t.Fatalf("output missing auth row:\n%s", out.String())
	}

	if !strings.Contains(line, "FAIL") {
		t.Fatalf("auth row %q does not FAIL", line)
	}

	if _, ok := doctorLine(out.String(), "db"); ok {
		t.Fatalf("output checks db, which the fixture does not enable:\n%s", out.String())
	}
}

// TestDoctorAdaptersRowSuggestsOnTypo pins the P4-1 fail-fast: a misspelled
// adapter fails the adapters row with a did-you-mean hint naming the
// registered adapter, instead of surfacing only as a resolve error.
func TestDoctorAdaptersRowSuggestsOnTypo(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeInspectFixture(t, dir, "zever.yaml", doctorTypoBatteryFixture)

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{ConfigPath: cfgPath, Out: &out}); err == nil {
		t.Fatal("runDoctorWith typo adapter: got nil error, want non-nil")
	}

	line, ok := doctorLine(out.String(), "adapters")
	if !ok {
		t.Fatalf("output missing adapters row:\n%s", out.String())
	}
	if !strings.Contains(line, "FAIL") {
		t.Fatalf("adapters row %q does not FAIL", line)
	}
	if !strings.Contains(out.String(), `did you mean "memory"`) {
		t.Fatalf("output lacks did-you-mean hint:\n%s", out.String())
	}
}

// TestDoctorNoProjectChecksAllWithNote pins the fallback: with no project
// config in the working directory every battery is checked and a note says so.
func TestDoctorNoProjectChecksAllWithNote(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	err := runDoctorWith(DoctorConfig{Out: &out})
	if err == nil {
		t.Fatal("runDoctorWith defaults: got nil error, want non-nil (auth/jwt has no secret)")
	}

	if got := out.String(); !strings.Contains(got, "no project config") {
		t.Fatalf("output lacks the all-batteries fallback note:\n%s", got)
	}

	for _, want := range []string{"config", "ai", "auth", "db"} {
		if _, ok := doctorLine(out.String(), want); !ok {
			t.Fatalf("fallback output missing battery %q:\n%s", want, out.String())
		}
	}
}

// TestDoctorUnknownFlagIsUsage pins exit code 2 for flag misuse.
func TestDoctorUnknownFlagIsUsage(t *testing.T) {
	err := runDoctor([]string{"--bogus-flag"})
	if err == nil {
		t.Fatal("runDoctor --bogus-flag: got nil error, want usage error")
	}

	if !errors.Is(err, errFlagUsage) {
		t.Fatalf("err = %v, want it to wrap errFlagUsage for exit 2", err)
	}
}

// TestDoctorServerReportUsedWhenAvailable proves the CLI execs the built
// server's --doctor mode when the project has an entrypoint: rows come from
// the server report, filtered to the enabled set.
func TestDoctorServerReportUsedWhenAvailable(t *testing.T) {
	ctx := t.Context()

	prev := runServerDoctor
	t.Cleanup(func() { runServerDoctor = prev })

	var gotEntry string

	runServerDoctor = func(gotCtx context.Context, entry string) (serverDoctorReport, error) {
		if gotCtx == nil || gotCtx.Err() != nil {
			t.Error("runServerDoctor: context is nil or done")
		}

		gotEntry = entry

		return serverDoctorReport{
			OK: false,
			Batteries: []serverDoctorBattery{
				{Name: "db", OK: false, Error: "server-side boom"},
				{Name: "ai", OK: true},
			},
		}, nil
	}

	dir := t.TempDir()
	writeInspectFixture(t, dir, "zever.yaml", "db:\n  adapter: sqlite\n")
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "server"), 0o750); err != nil {
		t.Fatalf("mkdir entrypoint: %v", err)
	}

	t.Chdir(dir)

	var out bytes.Buffer
	err := runDoctorWithContext(ctx, DoctorConfig{Out: &out})
	if err == nil {
		t.Fatal("runDoctorWith server FAIL row: got nil error, want non-nil")
	}

	if gotEntry != defaultServerEntry {
		t.Fatalf("server entry = %q, want %q", gotEntry, defaultServerEntry)
	}

	if _, ok := doctorLine(out.String(), "db"); !ok {
		t.Fatalf("output missing server-reported db row:\n%s", out.String())
	}

	if _, ok := doctorLine(out.String(), "ai"); ok {
		t.Fatalf("output checks ai, which the fixture does not enable:\n%s", out.String())
	}

	if got := out.String(); !strings.Contains(got, "cmd/server") {
		t.Fatalf("output lacks the server-source note:\n%s", got)
	}
}

// TestDoctorServerFallbackWithNote proves the CLI registry fallback: when the
// server exec fails, enabled batteries are still checked locally with a note.
func TestDoctorServerFallbackWithNote(t *testing.T) {
	ctx := t.Context()

	prev := runServerDoctor
	t.Cleanup(func() { runServerDoctor = prev })

	runServerDoctor = func(context.Context, string) (serverDoctorReport, error) {
		return serverDoctorReport{}, errors.New("go run exploded")
	}

	dir := t.TempDir()
	cfgPath := writeInspectFixture(t, dir, "zever.yaml", doctorTwoBatteryFixture)
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "server"), 0o750); err != nil {
		t.Fatalf("mkdir entrypoint: %v", err)
	}

	t.Chdir(dir)

	var out bytes.Buffer
	if err := runDoctorWithContext(ctx, DoctorConfig{ConfigPath: cfgPath, Out: &out}); err != nil {
		t.Fatalf("runDoctorWith fallback all-OK fixture: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "CLI registry") {
		t.Fatalf("output lacks the CLI-registry fallback note:\n%s", got)
	}

	names := doctorEnabledRows(t, out.String())
	if len(names) != 4 {
		t.Fatalf("checked batteries = %v, want exactly adapters/config/log/router:\n%s", names, out.String())
	}
}

// TestDoctorJSONProjectEnvelope keeps the P3-4 envelope over the new scope:
// two enabled batteries yield four data rows (config + adapters + 2) and ok:true.
func TestDoctorJSONProjectEnvelope(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	cfgPath := writeInspectFixture(t, dir, "zever.yaml", doctorTwoBatteryFixture)

	var human bytes.Buffer
	if err := runDoctorWith(DoctorConfig{ConfigPath: cfgPath, Out: &human, JSON: true}); err != nil {
		t.Fatalf("runDoctorWith JSON two-battery fixture: %v", err)
	}

	if human.String() != "" {
		t.Fatalf("human output = %q in JSON mode, want only the envelope", human.String())
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "doctor" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want ok doctor envelope", env)
	}

	data := envelopeData(t, env)

	rows, ok := data["batteries"].([]any)
	if !ok || len(rows) != 4 {
		t.Fatalf("data.batteries has %d rows, want 4 (adapters/config/log/router): %v", len(rows), data["batteries"])
	}

	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("battery row = %v, want object", r)
		}

		name, _ := row["name"].(string)
		if name != "config" && name != "adapters" && name != "log" && name != "router" {
			t.Fatalf("battery row %q not in the enabled set", name)
		}
	}
}

// TestDecodeServerDoctorReport pins the server --doctor JSON contract the CLI
// parses, including malformed output failing closed.
func TestDecodeServerDoctorReport(t *testing.T) {
	t.Parallel()

	rep, err := decodeServerDoctorReport([]byte(`{"ok":false,"batteries":[{"name":"db","ok":false,"error":"boom"}]}`))
	if err != nil {
		t.Fatalf("decodeServerDoctorReport: %v", err)
	}

	if rep.OK || len(rep.Batteries) != 1 || rep.Batteries[0].Name != "db" {
		t.Fatalf("report = %+v, want one failing db row", rep)
	}

	if _, err := decodeServerDoctorReport([]byte("not json")); err == nil {
		t.Fatal("decodeServerDoctorReport(garbage): got nil error, want non-nil")
	}
}

// TestServerTemplateHasDoctorMode pins the generated server contract: a
// --doctor flag that prints per-battery JSON without serving.
func TestServerTemplateHasDoctorMode(t *testing.T) {
	t.Parallel()

	for _, want := range []string{`"doctor"`, `batteries`, `runDoctor()`} {
		if !strings.Contains(serverTemplate, want) {
			t.Fatalf("serverTemplate lacks %q (--doctor mode)", want)
		}
	}
}

// TestDoctorChecksMatchServerTemplate guards the two battery tables against
// drift: every battery the CLI can check must resolve in the generated
// server's --doctor mode too.
func TestDoctorChecksMatchServerTemplate(t *testing.T) {
	t.Parallel()

	for name := range doctorChecksFor(nil, nil) {
		if name == "config" {
			continue
		}

		if !strings.Contains(serverTemplate, `"`+name+`"`) {
			t.Fatalf("serverTemplate lacks battery %q the CLI doctor checks", name)
		}
	}
}
