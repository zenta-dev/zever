package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

// writeSchemaFile writes name under dir/schema/. Owned
// by the generate wave.
func writeSchemaFile(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, "schema", name)

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write schema: %v", err)
	}
}

// TestRunGenerateWorkerWithNoSchema is the bootstrap case: a project with no
// .zen files yet still gets a compilable worker, just with no job stubs.
func TestRunGenerateWorkerWithNoSchema(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	if err := runGenerateWorker(nil); err != nil {
		t.Fatalf("runGenerateWorker: %v", err)
	}

	main := readGenerated(t, filepath.Join(dir, "cmd", "worker", "main.go"))

	if strings.Contains(main, "job.Register(") {
		t.Fatalf("expected no job stubs with no schema:\n%s", main)
	}

	if !strings.Contains(main, `Queues:      []string{"default"}`) {
		t.Fatalf("expected the default queue:\n%s", main)
	}

	// Nothing needs encoding/json when there are no schedules and no json
	// -typed job parameters.
	if strings.Contains(main, `"encoding/json"`) {
		t.Fatalf("encoding/json imported but unused:\n%s", main)
	}

	readGenerated(t, filepath.Join(dir, "internal", "app", "app.go"))

	assertGolden(t, "generate_worker_main_empty.golden", []byte(main))
}

func TestRunGenerateWorkerFromSchema(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	writeSchemaFile(t, dir, "shop.zen", `job ShipOrder(order_id: uuid, attempt_count: int32, extra: json) {
	queue: shipping
	retry: max_attempts(3), backoff(exponential, base: 30s)
}

job Sweep() {
	queue: low
}

schedule Nightly {
	cron: "0 0 * * *"
	dispatch: Sweep()
}
`)

	if err := runGenerateWorker(nil); err != nil {
		t.Fatalf("runGenerateWorker: %v", err)
	}

	main := readGenerated(t, filepath.Join(dir, "cmd", "worker", "main.go"))

	for _, fragment := range []string{
		`"example.com/shop/internal/app"`,
		`"example.com/shop/internal/service/jobs"`,
		`"encoding/json"`,
		`job.Register("ShipOrder", jobs.HandleShipOrder)`,
		`job.Register("Sweep", jobs.HandleSweep)`,
		`sched.Schedule(ctx, "0 0 * * *", "Sweep", json.RawMessage(` + "`{}`" + `))`,
		`sched.Start()`,
		// Every declared queue, sorted and de-duplicated.
		`Queues:      []string{"low", "shipping"}`,
	} {
		if !strings.Contains(main, fragment) {
			t.Fatalf("worker main.go lacks %q:\n%s", fragment, main)
		}
	}

	// The Args struct and handler body move into their own skip-if-exists
	// stub file per job, never inline in the always-regenerated main.go.
	if strings.Contains(main, "type ShipOrderArgs struct") || strings.Contains(main, "type SweepArgs struct") {
		t.Fatalf("worker main.go must not declare job Args structs inline:\n%s", main)
	}

	shipStub := readGenerated(t, filepath.Join(dir, "internal", "service", "jobs", "ship_order.go"))

	for _, fragment := range []string{
		"package jobs",
		"type ShipOrderArgs struct {",
		"OrderId      string          `json:\"order_id\"`",
		"AttemptCount int32           `json:\"attempt_count\"`",
		"Extra        json.RawMessage `json:\"extra\"`",
		"func HandleShipOrder(ctx context.Context, args ShipOrderArgs) error {",
		`apperror.New(apperror.Unimplemented, "ShipOrder job not implemented")`,
	} {
		if !strings.Contains(shipStub, fragment) {
			t.Fatalf("job stub lacks %q:\n%s", fragment, shipStub)
		}
	}

	readGenerated(t, filepath.Join(dir, "internal", "service", "jobs", "sweep.go"))

	assertGolden(t, "generate_worker_main_wired.golden", []byte(main))
	assertGolden(t, "generate_worker_job_stub.golden", []byte(shipStub))
}

// TestRunGenerateWorkerNeverOverwritesAnExistingJobStub proves the
// skip-if-exists contract: re-running after a developer has started
// implementing a job must leave it untouched, even with --force (--force
// only governs the always-regenerated main.go).
func TestRunGenerateWorkerNeverOverwritesAnExistingJobStub(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	writeSchemaFile(t, dir, "shop.zen", `job Sweep() {
	queue: low
}
`)

	if err := runGenerateWorker(nil); err != nil {
		t.Fatalf("runGenerateWorker: %v", err)
	}

	stubPath := filepath.Join(dir, "internal", "service", "jobs", "sweep.go")

	const handwritten = "package jobs\n\n// hand written job logic\n"

	if err := os.WriteFile(stubPath, []byte(handwritten), 0o600); err != nil {
		t.Fatalf("write handwritten stub: %v", err)
	}

	if err := runGenerateWorker([]string{"--force"}); err != nil {
		t.Fatalf("runGenerateWorker --force: %v", err)
	}

	if got := readFile(t, stubPath); got != handwritten {
		t.Fatalf("re-running generate worker overwrote a hand-implemented job stub: %q", got)
	}
}

func TestRunGenerateWorkerRefusesBrokenSchema(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	writeSchemaFile(t, dir, "broken.zen", "job Ship( {\n")

	if err := runGenerateWorker(nil); err == nil {
		t.Fatalf("expected an error for a schema that does not compile")
	}

	if _, err := os.Stat(filepath.Join(dir, "cmd", "worker", "main.go")); !os.IsNotExist(err) {
		t.Fatalf("a worker was scaffolded from a broken schema")
	}
}

func TestRunGenerateWorkerRefusesToClobberWithoutForce(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	if err := runGenerateWorker(nil); err != nil {
		t.Fatalf("runGenerateWorker: %v", err)
	}

	if err := runGenerateWorker(nil); err == nil {
		t.Fatalf("expected an error for an existing entrypoint")
	}

	if err := runGenerateWorker([]string{"--force"}); err != nil {
		t.Fatalf("runGenerateWorker --force: %v", err)
	}
}

func TestRunGenerateWorkerHonoursProjectConfig(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	const cfg = "project:\n  worker_entry: cmd/jobs\n  schema_dir: zen\n"

	if err := os.WriteFile(filepath.Join(dir, "zever.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write zever.yaml: %v", err)
	}

	path := filepath.Join(dir, "zen", "nested", "shop.zen")

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte("job Ship() {\n\tqueue: shipping\n}\n"), 0o600); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	if err := runGenerateWorker(nil); err != nil {
		t.Fatalf("runGenerateWorker: %v", err)
	}

	// The schema directory is walked recursively.
	main := readGenerated(t, filepath.Join(dir, "cmd", "jobs", "main.go"))

	if !strings.Contains(main, `job.Register("Ship"`) {
		t.Fatalf("nested schema file was not compiled:\n%s", main)
	}
}

func TestRunGenerateWorkerWithoutGoMod(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runGenerateWorker(nil); err == nil {
		t.Fatalf("expected an error outside a Go module")
	}
}

func TestCollectZenFilesRejectsNonDirectory(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "schema"), []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := collectZenFiles("test", "schema"); err == nil {
		t.Fatalf("expected an error when schema_dir is a file")
	}
}

func TestCompileSchemaDirWithMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	schema, err := compileSchemaDir("test", "schema")
	if err != nil {
		t.Fatalf("compileSchemaDir: %v", err)
	}

	if len(schema.Modules) != 0 {
		t.Fatalf("expected an empty schema, got %d module(s)", len(schema.Modules))
	}
}

func TestGoIdent(t *testing.T) {
	// goIdent is a plain word-capitalizer: it has no initialism table, so
	// order_id becomes OrderId, not OrderID.
	tests := map[string]string{
		"order_id":     "OrderId",
		"ship":         "Ship",
		"ShipOrder":    "ShipOrder",
		"a-b c":        "ABC",
		"":             "Args",
		"__":           "Args",
		"attempt_coun": "AttemptCoun",
	}

	for in, want := range tests {
		if got := goIdent(in); got != want {
			t.Fatalf("goIdent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGoScalarType(t *testing.T) {
	tests := []struct {
		in        ir.ScalarType
		want      string
		needsJSON bool
	}{
		{ir.TUUID, "string", false},
		{ir.TString, "string", false},
		{ir.TEnum, "string", false},
		{ir.TInt32, "int32", false},
		{ir.TInt64, "int64", false},
		{ir.TFloat32, "float32", false},
		{ir.TFloat64, "float64", false},
		{ir.TBool, "bool", false},
		{ir.TTimestamp, "time.Time", false},
		{ir.TDate, "time.Time", false},
		{ir.TBytes, "[]byte", false},
		{ir.TJSON, "json.RawMessage", true},
	}

	for _, tc := range tests {
		got, needsJSON := goScalarType(tc.in)
		if got != tc.want || needsJSON != tc.needsJSON {
			t.Fatalf("goScalarType(%v) = (%q, %v), want (%q, %v)", tc.in, got, needsJSON, tc.want, tc.needsJSON)
		}
	}
}

func TestDispatchArgsJSON(t *testing.T) {
	if got := dispatchArgsJSON(&ir.Schedule{}); got != "{}" {
		t.Fatalf("dispatchArgsJSON(no dispatch) = %q", got)
	}

	job := &ir.Job{Name: "Ship", Params: []*ir.Param{{Name: "order_id"}, {Name: "retry"}}}

	sched := &ir.Schedule{Dispatch: job, DispatchArgs: []any{"abc", int64(2)}}
	if got := dispatchArgsJSON(sched); got != `{"order_id":"abc","retry":2}` {
		t.Fatalf("dispatchArgsJSON = %q", got)
	}

	// Fewer arguments than parameters must not panic or over-read.
	short := &ir.Schedule{Dispatch: job, DispatchArgs: []any{"abc"}}
	if got := dispatchArgsJSON(short); got != `{"order_id":"abc"}` {
		t.Fatalf("dispatchArgsJSON(short) = %q", got)
	}

	// A backtick would break the raw string literal the template embeds this
	// in, so it degrades to an empty payload rather than emitting bad Go.
	tick := &ir.Schedule{Dispatch: job, DispatchArgs: []any{"a`b"}}
	if got := dispatchArgsJSON(tick); got != "{}" {
		t.Fatalf("dispatchArgsJSON(backtick) = %q", got)
	}
}

func TestBuildWorkerDataDefaultsToTheDefaultQueue(t *testing.T) {
	data := buildWorkerData("example.com/shop", &ir.Schema{})

	if len(data.Queues) != 1 || data.Queues[0] != "default" {
		t.Fatalf("Queues = %v", data.Queues)
	}

	if data.NeedsJSON {
		t.Fatalf("NeedsJSON should be false for an empty schema")
	}
}
