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

// doctorTwoBatteryFixture steers two zero-infra batteries via env. Doctor
// checks every known battery (service config is env-only, so no file lists
// an enabled set); the fixture only steers values, never scope. Adapters
// are code-wired, so the fixture names options only.
const doctorTwoBatteryFixture = "LOG_MIN_LEVEL=1\nDB_PATH=data/test.db\n"

// doctorFailingBatteryFixture leaves auth without its secret: jwt requires
// a 32+ byte secret, validated at Open, so resolution succeeds and the
// auth row FAILs with the AUTH_JWT_SECRET fix hint.
const doctorFailingBatteryFixture = "DB_PATH=data/test.db\n"

// doctorTypoBatteryFixture misspells an option key: the unknown key is close
// to a known field, so resolution fails the config row with a did-you-mean
// hint.
const doctorTypoBatteryFixture = "CACHE_MAXENTRIE=100\n"

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

// TestDoctorChecksAllBatteries pins the env-only scope: config values steer
// adapters, but every known battery is checked, never a file-listed subset.
func TestDoctorChecksAllBatteries(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", doctorTwoBatteryFixture)
	t.Chdir(dir)

	var out bytes.Buffer
	err := runDoctorWith(DoctorConfig{Out: &out})
	if err == nil {
		t.Fatal("runDoctorWith two-battery fixture: got nil error, want non-nil (auth/jwt has no secret)")
	}

	names := doctorEnabledRows(t, out.String())

	for _, want := range []string{"config", "log", "router", "db", "ai", "auth"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("checked batteries = %v, want row %q\noutput:\n%s", names, want, out.String())
		}
	}
}

// TestDoctorHumanFailsOnBatteryFailure pins the P3-3 exit-code fix: the human
// path returns a non-nil error (exit 1) when any battery fails, with
// no --strict flag needed.
func TestDoctorHumanFailsOnBatteryFailure(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", doctorFailingBatteryFixture)
	t.Chdir(dir)

	var out bytes.Buffer
	err := runDoctorWith(DoctorConfig{Out: &out})
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
}

// TestDoctorConfigRowSuggestsOnTypo pins the fail-fast: a misspelled
// option key fails the config row with a did-you-mean hint naming the real
// option, instead of resolving past it.
func TestDoctorConfigRowSuggestsOnTypo(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", doctorTypoBatteryFixture)
	t.Chdir(dir)

	var out bytes.Buffer
	err := runDoctorWith(DoctorConfig{Out: &out})
	if err == nil {
		t.Fatal("runDoctorWith typo option: got nil error, want non-nil")
	}
	if !strings.Contains(err.Error(), `did you mean "max_entries"`) {
		t.Fatalf("error lacks did-you-mean hint: %v", err)
	}
}

// TestDoctorNoProjectChecksAllWithNote pins the fallback: with no server
// entrypoint in the working directory every battery is checked via the CLI
// registry and a note says so.
func TestDoctorNoProjectChecksAllWithNote(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	err := runDoctorWith(DoctorConfig{Out: &out})
	if err == nil {
		t.Fatal("runDoctorWith defaults: got nil error, want non-nil (auth/jwt has no secret)")
	}

	if got := out.String(); !strings.Contains(got, "CLI wiring") {
		t.Fatalf("output lacks the CLI-registry note:\n%s", got)
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
// the server report.
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

	if _, ok := doctorLine(out.String(), "ai"); !ok {
		t.Fatalf("output missing server-reported ai row:\n%s", out.String())
	}

	if got := out.String(); !strings.Contains(got, "cmd/server") {
		t.Fatalf("output lacks the server-source note:\n%s", got)
	}
}

// TestDoctorServerFallbackWithNote proves the CLI registry fallback: when the
// server exec fails, batteries are still checked locally with a note.
func TestDoctorServerFallbackWithNote(t *testing.T) {
	ctx := t.Context()

	prev := runServerDoctor
	t.Cleanup(func() { runServerDoctor = prev })

	runServerDoctor = func(context.Context, string) (serverDoctorReport, error) {
		return serverDoctorReport{}, errors.New("go run exploded")
	}

	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", doctorTwoBatteryFixture)
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "server"), 0o750); err != nil {
		t.Fatalf("mkdir entrypoint: %v", err)
	}

	t.Chdir(dir)

	var out bytes.Buffer
	_ = runDoctorWithContext(ctx, DoctorConfig{Out: &out})

	if got := out.String(); !strings.Contains(got, "CLI wiring") {
		t.Fatalf("output lacks the CLI-registry fallback note:\n%s", got)
	}

	names := doctorEnabledRows(t, out.String())
	if len(names) == 0 {
		t.Fatalf("checked batteries = %v, want every known battery:\n%s", names, out.String())
	}
}

// TestDoctorJSONProjectEnvelope keeps the P3-4 envelope over the new scope:
// every known battery yields a data row and ok reflects the failing auth row.
func TestDoctorJSONProjectEnvelope(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", doctorTwoBatteryFixture)
	t.Chdir(dir)

	var human bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &human, JSON: true}); err == nil {
		t.Fatal("runDoctorWith JSON two-battery fixture: got nil error, want non-nil (auth/jwt has no secret)")
	}

	if human.String() != "" {
		t.Fatalf("human output = %q in JSON mode, want only the envelope", human.String())
	}

	env := mustDecodeEnvelope(t, jbuf)
	if env.OK || env.Command != "doctor" {
		t.Fatalf("envelope = %+v, want failing doctor envelope", env)
	}

	data := envelopeData(t, env)

	rows, ok := data["batteries"].([]any)
	if !ok || len(rows) == 0 {
		t.Fatalf("data.batteries has %d rows, want every known battery: %v", len(rows), data["batteries"])
	}

	seen := map[string]bool{}
	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("battery row = %v, want object", r)
		}

		name, _ := row["name"].(string)
		seen[name] = true
	}
	for _, want := range []string{"config", "log", "router", "db"} {
		if !seen[want] {
			t.Fatalf("battery row %q missing from the envelope", want)
		}
	}
}

// TestDoctorSurfacesNoPoolMismatch pins the new sharing model: batteries
// open their own pools (or share explicitly via NewFromDB in app code), so
// implicit config-driven pool sharing is gone and doctor prints no pool
// warnings for two batteries pointed at the same sqlite file. Unrelated
// batteries (ai/auth need credentials the fixture does not supply) still
// fail, so the command exits non-zero.
func TestDoctorSurfacesNoPoolMismatch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "shared.db")
	fixture := "CACHE_DSN=" + file + "\nDB_PATH=" + file + "\nDB_MAX_CONNS=5\n"
	writeInspectFixture(t, dir, ".env", fixture)
	t.Chdir(dir)

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err == nil {
		t.Fatal("runDoctorWith shared-file fixture: got nil error, want non-nil (unconfigured batteries fail)")
	}

	got := out.String()
	if strings.Contains(got, "pool: ") {
		t.Fatalf("output carries a pool mismatch warning, want none:\n%s", got)
	}

	if strings.Contains(got, "shared.db") {
		t.Fatalf("output leaks the path:\n%s", got)
	}
}

// TestDoctorJSONOmitsPoolWarnings pins the envelope contract: with no
// implicit sharing there are no warnings, so data carries no pool_warnings
// key at all.
func TestDoctorJSONOmitsPoolWarnings(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "shared.db")
	fixture := "CACHE_DSN=" + file + "\nDB_PATH=" + file + "\nDB_MAX_CONNS=5\n"
	writeInspectFixture(t, dir, ".env", fixture)
	t.Chdir(dir)

	var human bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &human, JSON: true}); err == nil {
		t.Fatal("runDoctorWith JSON shared-file fixture: got nil error, want non-nil (unconfigured batteries fail)")
	}

	env := mustDecodeEnvelope(t, jbuf)
	if env.OK || env.Command != "doctor" {
		t.Fatalf("envelope = %+v, want failing doctor envelope", env)
	}

	data := envelopeData(t, env)
	if warns, ok := data["pool_warnings"]; ok {
		t.Fatalf("data.pool_warnings = %v, want absent", warns)
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

// TestInterpretServerDoctor proves a built server that exits non-zero because
// a battery failed (its documented --doctor behavior) still yields its JSON
// report, and that a genuine crash reports the real error line rather than a
// leading log WARN.
func TestInterpretServerDoctor(t *testing.T) {
	t.Parallel()

	exitErr := errors.New("exit status 1")
	body := []byte(`{"ok":false,"batteries":[{"name":"ai","ok":false,"error":"boom"}]}`)

	rep, err := interpretServerDoctor(exitErr, body, "2026/10/15 WARN jwt: noise\nserver: batteries failed\n")
	if err != nil {
		t.Fatalf("interpretServerDoctor(exit 1 + report) error = %v, want nil", err)
	}

	if rep.OK || len(rep.Batteries) != 1 || rep.Batteries[0].Name != "ai" {
		t.Fatalf("report = %+v, want the failing ai row", rep)
	}

	_, err = interpretServerDoctor(exitErr, nil, "2026/10/15 WARN jwt: noise\npanic: boom\n")
	if err == nil {
		t.Fatal("interpretServerDoctor(crash) error = nil, want non-nil")
	}

	if strings.Contains(err.Error(), "WARN") || !strings.Contains(err.Error(), "panic: boom") {
		t.Fatalf("crash error = %q, want the last stderr line, not the WARN", err)
	}

	if _, err = interpretServerDoctor(nil, []byte("not json"), ""); err == nil {
		t.Fatal("interpretServerDoctor(clean exit, garbage) error = nil, want non-nil")
	}
}

// TestServerTemplateHasDoctorMode pins the generated server contract: the
// listen flags the launchers pass through (-addr/-grpc-addr), the grpcWired
// gate that keeps gRPC fail-closed on scaffolds without auth+permission,
// and the serve.Main delegation that owns boot (including --doctor).
func TestServerTemplateHasDoctorMode(t *testing.T) {
	t.Parallel()

	for _, want := range []string{
		`flag.String("addr"`,
		`flag.String("grpc-addr"`,
		"serve.Main(app.Providers(), modules, opts...)",
		"doctorMode(os.Args[1:])",
		"func doctorMode(args []string) bool {",
		"func grpcWired() bool {",
		"func serverPort(addr string) (string, error) {",
	} {
		if !strings.Contains(serverTemplate, want) {
			t.Fatalf("serverTemplate lacks %q (serve-library contract)", want)
		}
	}
}

// TestDoctorChecksMatchServerTemplate guards the two doctor tables against
// drift: every battery the generated server's --doctor mode reports on (the
// serve.Doctor table) must also be a battery the CLI doctor knows how to
// check, so a server report is never about a battery the CLI cannot
// explain. The serve-side table is pinned by serve's own tests; this is the
// parity direction the generated server depends on.
func TestDoctorChecksMatchServerTemplate(t *testing.T) {
	t.Parallel()

	cliChecks := doctorChecksFor(nil, nil)

	for _, name := range []string{
		"log",
		"router",
		"db",
		"observability",
		"queue",
		"auth",
		"permission",
		"scheduler",
	} {
		if _, ok := cliChecks[name]; !ok {
			t.Fatalf("serve doctor battery %q is not checked by the CLI doctor", name)
		}
	}
}
