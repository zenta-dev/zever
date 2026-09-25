package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/compile"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/naming"
)

const workerUsageBody = `Scaffolds the background worker entrypoint at <worker_entry>/main.go (default
cmd/worker, override with project.worker_entry), plus internal/app/app.go if
the project does not have one yet.

The worker and the scheduler run in one process over one shared queue, which
is what the in-process memory queue adapter requires; swapping the queue
adapter for redis/nats in zever.yaml is what makes it distributable, with no
code change in the generated file.

Every .zen file under <schema_dir> is compiled first, so the scaffold gets one
real job.Register call per declared job — with a payload struct built from
that job's declared parameters — and one sched.Add call per declared schedule.
With no schema files yet, the scaffold is generated with no job stubs; re-run
this command with --force after declaring jobs with ` + "`zever generate job`" + `.`

// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// var seam keeps behavior identical while enabling success/error coverage.
var promptConfirmForWorker = promptConfirm

func printWorkerUsage(fset *flag.FlagSet) {
	header := title("zever generate worker") + dim(" — scaffold background worker")
	usage := bold("Usage:") + "  " + cmd("zever generate worker") + dim(" [--force]") + dim("  •  -i/--interactive for guided prompts")

	printBoxedUsage(fset, header, usage, workerUsageBody, []usageExample{
		{command: "zever generate worker"},
		{command: "zever generate worker --force"},
		{command: "zever generate worker -i", comment: "  # confirm overwrite if exists"},
	}, "re-run with --force after adding jobs")
}

// workerJob is one job.Register call in the generated worker, and also the
// data jobStubTemplate renders one skip-if-exists implementation file from.
type workerJob struct {
	Name      string
	ArgsType  string
	Handler   string // "Handle" + PascalCase(Name), the stub package's exported function name.
	Fields    []workerField
	Imports   []string
	NeedsJSON bool // this job's own Args struct needs "encoding/json" (a json.RawMessage field).
	NeedsTime bool // this job's own Args struct needs "time" (a time.Time field).
}

type workerField struct {
	GoName   string
	GoType   string
	JSONName string
	Import   string
}

// workerSchedule is one sched.Add call in the generated worker.
type workerSchedule struct {
	Name string
	Cron string
	Job  string
	Args string // a JSON object literal
}

// workerData is the worker template's input.
type workerData struct {
	ModulePath string
	Jobs       []workerJob
	Schedules  []workerSchedule
	Queues     []string
	NeedsJSON  bool
}

// GenerateWorkerConfig is the pure input to GenerateWorker: resolved project
// paths plus the force flag.
type GenerateWorkerConfig struct {
	ModulePath  string
	SchemaDir   string
	WorkerEntry string
	Force       bool
	Stdout      io.Writer
	Stderr      io.Writer
}

// GenerateWorkerResult names everything GenerateWorker wrote.
type GenerateWorkerResult struct {
	Entrypoint string
	AppCreated bool
	Stubs      []string
}

// GenerateWorker compiles the schema directory, renders the worker
// entrypoint (and job stubs), and writes them to disk. It performs no flag
// parsing and no prompting.
func GenerateWorker(cfg GenerateWorkerConfig) (GenerateWorkerResult, error) {
	const tag = "zever generate worker"

	var res GenerateWorkerResult

	schema, err := compileSchemaDir(tag, cfg.SchemaDir)
	if err != nil {
		return res, err
	}

	appCreated, err := ensureAppPackage(tag)
	if err != nil {
		return res, err
	}

	res.AppCreated = appCreated

	data := buildWorkerData(cfg.ModulePath, schema)

	content, err := renderGoFile(tag, "worker main.go", workerTemplate, data)
	if err != nil {
		return res, err
	}

	path := filepath.Join(cfg.WorkerEntry, "main.go")

	if writeErr := writeScaffold(tag, path, content, cfg.Force); writeErr != nil {
		return res, writeErr
	}

	res.Entrypoint = path

	stubsWritten, err := writeJobStubs(tag, "", data)
	if err != nil {
		return res, err
	}

	res.Stubs = stubsWritten

	if stdout := cfg.Stdout; stdout != nil {
		if appCreated {
			_, _ = fmt.Fprintf(stdout, "%s %s\n", successMark(), success("scaffolded ")+cyan(filepath.Join("internal", "app", "app.go")))
		}

		_, _ = fmt.Fprintf(stdout, "%s %s %s\n", successMark(), success("scaffolded worker entrypoint at"), cyan(path))

		for _, p := range stubsWritten {
			_, _ = fmt.Fprintf(stdout, "%s %s %s\n", successMark(), success("scaffolded job stub at"), cyan(p))
		}
	}

	if stderr := cfg.Stderr; stderr != nil && shouldShowHint() {
		_, _ = fmt.Fprintln(stderr, formatHint("next: zever queue:work or zever dev"))
	}

	return res, nil
}

func runGenerateWorker(args []string) error {
	project, modulePath, forceVal, err := parseEntrypointFlags(args, "generate worker", printWorkerUsage,
		func(p ProjectConfig) string { return p.WorkerEntry }, promptConfirmForWorker)
	if err != nil {
		return err
	}

	_, err = GenerateWorker(GenerateWorkerConfig{
		ModulePath:  modulePath,
		SchemaDir:   project.SchemaDir,
		WorkerEntry: project.WorkerEntry,
		Force:       forceVal,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	})

	return err
}

// compileSchemaDir compiles every .zen file under dir into an *ir.Schema. A
// missing or empty schema directory is not an error: it yields an empty
// schema, and the caller scaffolds a worker with no job stubs.
func compileSchemaDir(tag, dir string) (*ir.Schema, error) {
	files, err := collectZenFiles(tag, dir)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return &ir.Schema{}, nil
	}

	result, diags := compile.Compile(files)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	if diags.HasErrors() {
		return nil, fmt.Errorf("%s: %d schema file(s) failed to compile", tag, len(files))
	}

	return result.Schema, nil
}

// collectZenFiles walks dir recursively (via the shared walkZenFiles helper
// in prompt.go) and reads every .zen file into the map shape compile.Compile
// expects.
func collectZenFiles(tag, dir string) (map[string]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil //nolint:nilnil // absent schema dir means "no jobs": callers treat nil map as empty
		}

		return nil, fmt.Errorf("%s: stat %q: %w", tag, dir, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%s: schema_dir %q is not a directory", tag, dir)
	}

	paths, walkErr := walkZenFiles(dir)
	if walkErr != nil {
		return nil, fmt.Errorf("%s: scan %q: %w", tag, dir, walkErr)
	}

	files := make(map[string]string, len(paths))

	for _, path := range paths {
		data, err := os.ReadFile(path) //nolint:gosec // developer-supplied schema directory
		if err != nil {
			return nil, fmt.Errorf("%s: scan %q: %w", tag, dir, err)
		}

		files[path] = string(data)
	}

	return files, nil
}

// buildWorkerData flattens the resolved schema into the template's view:
// one stub per job, one sched.Add per schedule, and the union of every
// declared queue name for the worker's Queues list.
func buildWorkerData(modulePath string, schema *ir.Schema) workerData {
	data := workerData{ModulePath: modulePath}

	queues := map[string]bool{}

	for _, module := range schema.Modules {
		for _, job := range module.Jobs {
			data.Jobs = append(data.Jobs, buildWorkerJob(job))

			if job.Queue != "" {
				queues[job.Queue] = true
			}
		}

		for _, sched := range module.Schedules {
			if sched.Dispatch == nil {
				continue
			}

			data.Schedules = append(data.Schedules, workerSchedule{
				Name: sched.Name,
				Cron: sched.Cron,
				Job:  sched.Dispatch.Name,
				Args: dispatchArgsJSON(sched),
			})

			data.NeedsJSON = true
		}
	}

	sort.Slice(data.Jobs, func(i, j int) bool { return data.Jobs[i].Name < data.Jobs[j].Name })
	sort.Slice(data.Schedules, func(i, j int) bool { return data.Schedules[i].Name < data.Schedules[j].Name })

	for queue := range queues {
		data.Queues = append(data.Queues, queue)
	}

	sort.Strings(data.Queues)

	if len(data.Queues) == 0 {
		data.Queues = []string{"default"}
	}

	return data
}

func buildWorkerJob(job *ir.Job) workerJob {
	ident := goIdent(job.Name)
	wj := workerJob{Name: job.Name, ArgsType: ident + "Args", Handler: "Handle" + ident}

	for _, param := range job.Params {
		goType, needsJSON := goScalarType(param.Type.Scalar)

		wj.Fields = append(wj.Fields, workerField{
			GoName:   goIdent(param.Name),
			GoType:   goType,
			JSONName: param.Name,
		})

		wj.NeedsJSON = wj.NeedsJSON || needsJSON
		wj.NeedsTime = wj.NeedsTime || goType == "time.Time"
	}

	return wj
}

// dispatchArgsJSON renders a schedule's dispatch arguments as the JSON object
// the job handler will decode: the target job's parameter names zipped with
// the literal values the schedule passes.
func dispatchArgsJSON(sched *ir.Schedule) string {
	if sched.Dispatch == nil || len(sched.DispatchArgs) == 0 {
		return "{}"
	}

	payload := map[string]any{}

	for i, param := range sched.Dispatch.Params {
		if i >= len(sched.DispatchArgs) {
			break
		}

		payload[param.Name] = sched.DispatchArgs[i]
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}

	// The template embeds this in a raw string literal, so a backtick would
	// break the generated Go. No JSON encoding of a .zen literal can produce
	// one today, but be defensive rather than emit unparseable source.
	if strings.Contains(string(encoded), "`") {
		return "{}"
	}

	return string(encoded)
}

// goScalarType maps a DSL scalar to the Go type a job payload field uses,
// mirroring the zenorm backend's own scalar mapping. "time" is always
// imported by the worker template (shutdownGrace), so only the encoding/json
// import is conditional.
func goScalarType(t ir.ScalarType) (goType string, needsJSON bool) {
	switch t {
	case ir.TUUID, ir.TString, ir.TEnum:
		return "string", false
	case ir.TInt32:
		return "int32", false
	case ir.TInt64:
		return "int64", false
	case ir.TFloat32:
		return "float32", false
	case ir.TFloat64:
		return "float64", false
	case ir.TBool:
		return "bool", false
	case ir.TTimestamp, ir.TDate:
		return "time.Time", false
	case ir.TBytes:
		return "[]byte", false
	case ir.TJSON:
		return "json.RawMessage", true
	}

	return "any", false
}

// goIdent turns a snake_case DSL name into an exported Go identifier.
func goIdent(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' || r == ' ' })

	var b strings.Builder

	for _, part := range parts {
		if part == "" {
			continue
		}

		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}

	if b.Len() == 0 {
		return "Args"
	}

	return b.String()
}

// jobStubRoot is the fixed directory job implementations are scaffolded
// under, one file per job (by its schema name) -- mirrors serverStubRoot's
// role for service implementations (generate_server.go): mechanical wiring
// (job.Register calls, the Args struct) stays in the always-regenerated
// worker main.go's package, while the actual handler body lives here, once,
// never overwritten.
const jobStubRoot = "internal/service/jobs"

// writeJobStubs writes jobStubTemplate's output for every job in data that
// has no stub file yet, under outDir/jobStubRoot (outDir is "" for the
// common in-place case; `zever extract` passes its own OutDir). Returns the
// paths actually written. Skipped silently (no error, no overwrite) when the
// file already exists -- see writeStubIfMissing.
func writeJobStubs(tag, outDir string, data workerData) ([]string, error) {
	var written []string

	for _, job := range data.Jobs {
		if isTraversalName(job.Name) {
			return written, fmt.Errorf("%s: %w: %q", tag, ErrPathTraversal, job.Name)
		}

		content, err := renderGoFile(tag, "job stub "+job.Name, jobStubTemplate, job)
		if err != nil {
			return written, fmt.Errorf("%s: render job stub for %s: %w", tag, job.Name, err)
		}

		path := filepath.Join(outDir, jobStubRoot, naming.SnakeCase(job.Name)+".go")

		ok, err := writeStubIfMissing(tag, path, content)
		if err != nil {
			return written, err
		}

		if ok {
			written = append(written, path)
		}
	}

	return written, nil
}

// jobStubTemplate renders one job's Args struct and a starting-point
// Handle<Job> implementation into its own file in the "jobs" package.
// Deliberately does NOT say "DO NOT EDIT" -- unlike workerTemplate, this
// file is written once and meant to be filled in by hand.
const jobStubTemplate = `// Code generated by the zever DSL as a starting point.
// Fill in the real job logic below -- this file is written once and never
// regenerated, so it is safe to edit freely.
package jobs

import (
	"context"
{{- if .NeedsJSON}}
	"encoding/json"
{{- end}}
{{- if .NeedsTime}}
	"time"
{{- end}}

	"github.com/zenta-dev/zever/apperror"
)

// {{.ArgsType}} is the payload of the {{.Name}} job declared in the schema.
type {{.ArgsType}} struct {
{{- range .Fields}}
	{{.GoName}} {{.GoType}} ` + "`json:\"{{.JSONName}}\"`" + `
{{- end}}
}

// {{.Handler}} is a starting-point implementation of the {{.Name}} job
// declared in the schema. Replace this with real logic.
func {{.Handler}}(ctx context.Context, args {{.ArgsType}}) error {
	return apperror.New(apperror.Unimplemented, "{{.Name}} job not implemented")
}
`

// workerTemplate is a direct transcription of
// examples/todo/cmd/worker/main.go's structure, parameterized on the target
// project's module path and on the jobs and schedules its schema declares.
const workerTemplate = `// Command worker runs this project's background job worker and its embedded
// scheduler in one process.
//
// Scaffolded by ` + "`zever generate worker`" + `. The scheduler dispatches onto the
// queue and the worker consumes from it, so both must share one queue: with
// the in-process memory adapter that means one process. Swapping the queue
// adapter for redis/nats in zever.yaml is what makes this distributable, with
// no code change here.
package main

import (
	"context"
{{- if .NeedsJSON}}
	"encoding/json"
{{- end}}
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenta-dev/zever/job"

	"{{.ModulePath}}/internal/app"
{{- if .Jobs}}
	"{{.ModulePath}}/internal/service/jobs"
{{- end}}
)

const (
	shutdownGrace = 10 * time.Second
	concurrency   = 4
)

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString("worker: " + err.Error() + "\n")

		os.Exit(1)
	}
}

func run() error {
	c, err := app.New()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()

		_ = c.Close(closeCtx)
	}()

	logger, err := c.Log()
	if err != nil {
		return err
	}
{{- if .Jobs}}

	// One handler per job declared in the schema, implemented in its own
	// skip-if-exists file under internal/service/jobs/ (never overwritten
	// once it exists). A job that is not registered in this process is never
	// executed, so keep this list in step with the schema (re-run
	// ` + "`zever generate worker --force`" + ` after adding one -- existing job
	// implementations are left untouched).
{{- range .Jobs}}
	job.Register("{{.Name}}", jobs.{{.Handler}})
{{- end}}
{{- else}}

	// No jobs are declared in the schema yet. Declare one with
	// ` + "`zever generate job <module> <Name>`" + ` and re-run
	// ` + "`zever generate worker --force`" + ` to get its handler stub here.
{{- end}}

	q, err := c.Queue()
	if err != nil {
		return err
	}

	sched, err := c.Scheduler()
	if err != nil {
		return err
	}
{{- range .Schedules}}

	// schedule {{.Name}}
	if _, err := sched.Schedule(ctx, {{printf "%q" .Cron}}, {{printf "%q" .Job}}, json.RawMessage(` + "`{{.Args}}`" + `)); err != nil {
		return err
	}
{{- end}}

	if err := sched.Start(); err != nil {
		return err
	}

	logger.Info().Int("jobs", {{len .Jobs}}).Int("schedules", {{len .Schedules}}).Msg("worker started")

	worker := &job.Worker{
		Q:           q,
		Queues:      []string{ {{- range $i, $q := .Queues}}{{if $i}}, {{end}}{{printf "%q" $q}}{{end -}} },
		Concurrency: concurrency,
	}

	return worker.Run(ctx)
}
`
