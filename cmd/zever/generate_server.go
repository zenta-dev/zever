package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/internal/dsl/backend/gogen"
	"github.com/zenta-dev/zever/internal/dsl/compile"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/naming"
)

// --- shared scaffolding helpers (server, worker and seed all use these) ---

// errNoGoMod is returned when a scaffold command runs outside a Go module.
var errNoGoMod = errors.New("zever generate: no go.mod in the working directory (run this from your project root)")

// goModulePath reads the module path out of ./go.mod. The scaffolds import
// the target project's own internal/app package, so they must be written
// against that project's module path, not zever's.
//
// go.mod is parsed by hand rather than with golang.org/x/mod: one line of
// string handling is not worth a new dependency on a CLI that has stayed
// dependency-free.
// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// var seam keeps behavior identical while enabling success/error coverage.
var promptConfirmForServer = promptConfirm

func goModulePath() (string, error) {
	f, err := os.Open("go.mod")
	if err != nil {
		if os.IsNotExist(err) {
			return "", errNoGoMod
		}

		return "", fmt.Errorf("zever generate: open go.mod: %w", err)
	}

	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		path, ok := strings.CutPrefix(line, "module")
		if !ok {
			continue
		}

		path = strings.TrimSpace(path)
		if path == "" || path == line {
			continue
		}

		// A quoted module path is legal go.mod syntax.
		return strings.Trim(path, `"`), nil
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("zever generate: read go.mod: %w", err)
	}

	return "", errors.New("zever generate: go.mod declares no module path")
}

// renderGoFile executes tmpl against data and gofmts the result, so every
// scaffolded file is gofmt-clean the moment it lands on disk.
func renderGoFile(tag, name, tmpl string, data any) ([]byte, error) {
	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("%s: parse %s template: %w", tag, name, err)
	}

	var buf bytes.Buffer

	if execErr := t.Execute(&buf, data); execErr != nil {
		return nil, fmt.Errorf("%s: render %s template: %w", tag, name, execErr)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("%s: generated %s is not valid Go: %w", tag, name, err)
	}

	return formatted, nil
}

// writeScaffold writes content to path, creating parent directories. It
// refuses to clobber an existing file unless force is set — a scaffold is a
// starting point, and silently overwriting hand-written code would be a
// data-loss bug.
func writeScaffold(tag, path string, content []byte, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s: %q already exists (pass --force to overwrite)", tag, path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("%s: stat %q: %w", tag, path, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("%s: mkdir %q: %w", tag, filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil { //nolint:gosec // generated source, not a secret
		return fmt.Errorf("%s: write %q: %w", tag, path, err)
	}

	return nil
}

// coreBatteries is the fixed, always-scaffolded battery set: every name here
// is one the generated entrypoints (serverTemplate, workerTemplate,
// seedTemplate) resolve unconditionally, or opt into once configured
// (ratelimit -- see rateLimitConfigured's doc comment for why its adapter
// must still always be registered). This is the floor a battery selection
// never goes below, whether it comes from `zever new`'s wizard/flags or
// ensureAppPackage's no-selection-context fallback.
var coreBatteries = []string{
	"auth", "db", "log", "observability", "permission", "queue", "ratelimit", "router", "scheduler",
}

// batterySelection is one battery+adapter pair, the single shape both
// renderZeverYaml (new.go) and appTemplate render from, so zever.yaml and
// app.go's blank imports can never drift apart.
type batterySelection struct {
	Battery string
	Adapter string
}

// batteryImportPath returns b's adapter package import path. Mechanical and
// exception-free across every battery/adapter pair in this module: every
// adapter lives at "<battery>/<adapter>" (e.g. cache/memory, db/sqlite,
// search/postgres), matching config.Default()'s own battery/adapter naming
// exactly.
func batteryImportPath(b batterySelection) string {
	return "github.com/zenta-dev/zever/" + b.Battery + "/" + b.Adapter
}

// coreBatterySelections resolves coreBatteries against config.Default()'s
// adapter picks -- the minimum app.go needs so every generated entrypoint's
// c.<Battery>() call resolves, used whenever nothing more specific (a
// `zever new` battery selection) is available.
//
// Adaptation note: zever's config is typed (Service[T] fields), so the picks
// come from named fields rather than zen-go's cfg.Get(name).Adapter map.
func coreBatterySelections() []batterySelection {
	cfg := config.Default()

	byName := map[string]string{
		"auth":          cfg.Auth.Adapter,
		"db":            cfg.DB.Adapter,
		"log":           cfg.Log.Adapter,
		"observability": cfg.Observability.Adapter,
		"permission":    cfg.Permission.Adapter,
		"queue":         cfg.Queue.Adapter,
		"ratelimit":     cfg.RateLimit.Adapter,
		"router":        cfg.Router.Adapter,
		"scheduler":     cfg.Scheduler.Adapter,
	}

	sel := make([]batterySelection, 0, len(coreBatteries))

	for _, name := range coreBatteries {
		sel = append(sel, batterySelection{Battery: name, Adapter: byName[name]})
	}

	return sel
}

// appTemplateData is appTemplate's render input: Imports are already-resolved
// adapter import paths, sorted for deterministic output.
type appTemplateData struct {
	Imports []string
}

// renderAppContent renders appTemplate for exactly the given battery
// selection -- the one place that turns a []batterySelection into app.go's
// content, shared by ensureAppPackage and `zever new`'s writeNewProject so
// the two callers can never format the import block differently.
func renderAppContent(tag string, selections []batterySelection) ([]byte, error) {
	imports := make([]string, 0, len(selections))
	for _, s := range selections {
		imports = append(imports, batteryImportPath(s))
	}

	sort.Strings(imports)

	return renderGoFile(tag, "app.go", appTemplate, appTemplateData{Imports: imports})
}

// ensureAppPackage scaffolds internal/app/app.go when the target project has
// none. Every generated entrypoint calls app.New(), mirroring
// examples/todo/internal/app: the container's adapter registries only learn
// about an adapter once that adapter package's init() has run, so one place
// per project owns those blank imports. Called with no battery-selection
// context (e.g. `zever generate server` run without a prior `zever new`), so
// it renders with just coreBatterySelections -- the floor every generated
// entrypoint needs, not a developer's fuller pick.
func ensureAppPackage(tag string) (created bool, err error) {
	path := filepath.Join("internal", "app", "app.go")

	if _, statErr := os.Stat(path); statErr == nil {
		return false, nil
	} else if !os.IsNotExist(statErr) {
		return false, fmt.Errorf("%s: stat %q: %w", tag, path, statErr)
	}

	content, err := renderAppContent(tag, coreBatterySelections())
	if err != nil {
		return false, err
	}

	if err := writeScaffold(tag, path, content, false); err != nil {
		return false, err
	}

	return true, nil
}

// --- generate server ---

const serverUsageBody = `Scaffolds the HTTP+gRPC server entrypoint at <server_entry>/main.go (default
cmd/server, override with project.server_entry in zever.yaml/.toml/.json),
plus internal/app/app.go if the project does not have one yet. The
scaffolded entrypoint serves HTTP on -addr (default :8080) and gRPC on
-grpc-addr (default :9090), both with graceful shutdown.

Pass .zen schema files (or rely on auto-discovery under the project's schema
dir) and the scaffolded main.go actually registers every declared module's
HTTP routes and gRPC server via its generated RegisterModule -- no more
manual TODOs. For every service with no stub implementation yet, a
compiling (Unimplemented) starting-point stub is also written under
internal/service/<module>/ -- never overwritten once it exists, so re-running
after you've started implementing a service is always safe. Requires the
schema to have already been compiled with the gogen backend (zever compile
--backend gogen) into the same --out directory.`

func printServerUsage(fs *flag.FlagSet) { //nolint:dupl
	header := title("zever generate server") + dim(" — scaffold HTTP server")
	usage := bold("Usage:") + "  " + cmd("zever generate server") + dim(" [--force] [--out DIR] [files...]") + dim("  •  -i/--interactive for guided prompts")

	body := joinLines(
		header,
		"",
		usage,
		"",
		serverUsageBody,
		"",
		bold("Flags:"),
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(fs.Output(), box(body))
	} else {
		_, _ = fmt.Fprintln(fs.Output(), body)
	}

	fs.PrintDefaults()
	_, _ = fmt.Fprintln(fs.Output(), "")
	_, _ = fmt.Fprintln(fs.Output(), dim("Examples:"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever generate server"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever generate server schema/v1/iam/*.zen"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever generate server --force"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever generate server -i")+dim("  # confirm overwrite if exists"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(fs.Output(), formatHint("entrypoint plus internal/app/app.go if missing"))
	}
}

// GenerateServerConfig is the pure input to GenerateServer: resolved project
// paths plus the flags. RateLimitEnabled gates the rate-limit wiring (see
// rateLimitConfigured); callers obtain it from the project config, not flags.
type GenerateServerConfig struct {
	ModulePath       string
	OutDir           string
	SchemaDir        string
	ServerEntry      string
	SchemaFiles      []string
	Force            bool
	RateLimitEnabled bool
	Stdout           io.Writer
	Stderr           io.Writer
}

// GenerateServerResult names everything GenerateServer wrote.
type GenerateServerResult struct {
	Entrypoint string
	AppCreated bool
	Stubs      []string
	Modules    int
}

// GenerateServer renders the server entrypoint (and service stubs) from
// resolved inputs and writes them to disk. It performs no flag parsing and
// no prompting.
func GenerateServer(cfg GenerateServerConfig) (GenerateServerResult, error) {
	const tag = "zever generate server"

	var res GenerateServerResult

	appCreated, err := ensureAppPackage(tag)
	if err != nil {
		return res, err
	}

	res.AppCreated = appCreated

	data, err := loadServerData(cfg.ModulePath, cfg.OutDir, cfg.SchemaDir, cfg.SchemaFiles, cfg.RateLimitEnabled)
	if err != nil {
		return res, err
	}

	res.Modules = len(data.Modules)

	content, err := renderGoFile(tag, "server main.go", serverTemplate, data)
	if err != nil {
		return res, err
	}

	path := filepath.Join(cfg.ServerEntry, "main.go")

	if writeErr := writeScaffold(tag, path, content, cfg.Force); writeErr != nil {
		return res, writeErr
	}

	res.Entrypoint = path

	stubsWritten, err := writeServiceStubs(tag, data)
	if err != nil {
		return res, err
	}

	res.Stubs = stubsWritten

	if stdout := cfg.Stdout; stdout != nil {
		if appCreated {
			_, _ = fmt.Fprintf(stdout, "%s %s\n", successMark(), success("scaffolded ")+cyan(filepath.Join("internal", "app", "app.go")))
		}

		_, _ = fmt.Fprintf(stdout, "%s %s %s\n", successMark(), success("scaffolded server entrypoint at"), cyan(path))

		for _, p := range stubsWritten {
			_, _ = fmt.Fprintf(stdout, "%s %s %s\n", successMark(), success("scaffolded service stub at"), cyan(p))
		}
	}

	if stderr := cfg.Stderr; stderr != nil && shouldShowHint() {
		if len(data.Modules) == 0 {
			_, _ = fmt.Fprintln(stderr, formatHint("pass .zen schema files to wire routes/gRPC automatically, e.g. zever generate server schema/*.zen"))
		} else {
			_, _ = fmt.Fprintln(stderr, formatHint("next: fill in the scaffolded service stub(s), then zever serve"))
		}
	}

	return res, nil
}

func runGenerateServer(args []string) error { //nolint:gocyclo
	args = peelInteractive(args)
	fs := flag.NewFlagSet("generate server", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite the entrypoint if it already exists")
	outDir := fs.String("out", "./generated", "output directory the schema was compiled into (must match `zever compile --out`)")
	// coverageProof: no local -i/--interactive flags; peelInteractive
	// strips them before Parse and sets interactiveMode globally.

	fs.Usage = func() {
		printServerUsage(fs)
	}

	posArgs, err := flexibleParse(fs, args)
	if err != nil {
		return err
	}

	project, err := loadProjectConfig()
	if err != nil {
		return err
	}

	modulePath, err := goModulePath()
	if err != nil {
		return err
	}

	rateLimitEnabled := rateLimitConfigured()

	forceVal := *force
	if !forceVal && isInteractiveTerminal() {
		if _, statErr := os.Stat(filepath.Join(project.ServerEntry, "main.go")); statErr == nil {
			ok, perr := promptConfirmForServer(fmt.Sprintf("%q already exists — overwrite?", filepath.Join(project.ServerEntry, "main.go")), "Yes, overwrite")
			if perr != nil {
				return perr
			}

			if ok {
				forceVal = true
			}
		}
	}

	_, err = GenerateServer(GenerateServerConfig{
		ModulePath:       modulePath,
		OutDir:           *outDir,
		SchemaDir:        project.SchemaDir,
		ServerEntry:      project.ServerEntry,
		SchemaFiles:      posArgs,
		Force:            forceVal,
		RateLimitEnabled: rateLimitEnabled,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
	})

	return err
}

// serverServiceData is one service's wiring info for the server template:
// enough to construct its stub implementation and pass it into the
// generated ModuleImpls literal.
type serverServiceData struct {
	// Name is the Pascal-case service name, e.g. "TaskService" -- also the
	// ModuleImpls field name RegisterModule expects.
	Name string
	// ImplVar is the local variable name main() binds the constructed stub
	// to, e.g. "taskServiceImpl".
	ImplVar string
	// Constructor is GenerateServiceStub's emitted constructor function
	// name, e.g. "NewTaskServiceImpl".
	Constructor string
	// ir is the resolved service GenerateServiceStub renders against.
	// Unexported: never read by serverTemplate, only by writeServiceStubs.
	ir *ir.Service
}

// serverModuleData is one module's wiring info for the server template.
type serverModuleData struct {
	// Alias is the import alias for the module's generated gogen package,
	// e.g. "genapp".
	Alias string
	// ImportPath is that generated package's Go import path.
	ImportPath string
	// NeedsAuthz mirrors gogen.ModuleNeedsAuthz(m): whether RegisterModule
	// takes auth.Auth/permission.Checker params.
	NeedsAuthz bool
	// ImplAlias is the import alias for this module's stub-implementation
	// package, e.g. "appimpl".
	ImplAlias string
	// ImplImportPath is that stub package's Go import path.
	ImplImportPath string
	// ImplDir is that stub package's on-disk directory, relative to the
	// project root (e.g. "internal/service/app").
	ImplDir  string
	Services []serverServiceData
}

// serverData is the full data serverTemplate renders against.
type serverData struct {
	ModulePath string
	Modules    []serverModuleData
	// RateLimitEnabled is true only when the project's own zever.yaml (or
	// equivalent) configures a real rate/burst for the "ratelimit" battery
	// -- see rateLimitConfigured's doc comment for why this can't simply be
	// "always resolve c.RateLimit()".
	RateLimitEnabled bool
}

// rateLimitConfigured reports whether the calling project has configured a
// real rate limit for the "ratelimit" battery. Unlike most batteries,
// ratelimit/memory's zero-infra adapter still requires an explicit
// rate/burst and errors without one, so unconditionally resolving
// c.RateLimit() in the generated main.go would break every project that
// hasn't configured a limit. Rate-limit wiring is instead emitted only when
// the config says a limit is actually set -- add
// `ratelimit: {options: {rate: ..., burst: ...}}` to zever.yaml and
// re-run `zever generate server --force` to turn it on, no other wiring
// needed.
//
// Adaptation note: zever's config is typed, so this reads
// cfg.RateLimit.Options.Rate directly instead of zen-go's
// cfg.Get("ratelimit").Options["rate"] JSON-round-tripped map lookup.
// A second zever difference: zever.yaml may carry a "project" table that
// config.Load's strict merge rejects (see project.go), so an unreadable
// config falls back to config.Default() instead of failing generation --
// the real error still surfaces at serve time. zever's defaults ship a real
// limit (rate 10, burst 20), so the fallback wires the limiter on.
func rateLimitConfigured() bool {
	cfg, err := config.Load("")
	if err != nil {
		cfg = config.Default()
	}

	return cfg.RateLimit.Options.Rate > 0
}

// serverStubRoot is the fixed directory service-implementation stubs are
// scaffolded under, one subdirectory per module (by its gogen output dir
// name) -- kept separate from internal/app (the container-wiring package
// every project already has) to avoid a confusing "internal/app/app" nested
// path for the common single, unnamed module case.
const serverStubRoot = "internal/service"

// genImportPath returns the Go import path of the gogen package generated
// for a module at output-relative directory dir, given outDir (the same
// --out a prior `zever compile` used) -- mirrors computeBackendRoots'
// (compile.go) own outDir-to-import-path formula exactly.
func genImportPath(modulePath, outDir, dir string) string {
	rel := filepath.ToSlash(filepath.Clean(outDir))
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")

	return modulePath + "/" + rel + "/gogen/" + dir
}

// lowerFirst lowercases s's first rune, for turning a Pascal-case service
// name into a local variable name (e.g. "TaskService" -> "taskService").
func lowerFirst(s string) string {
	if s == "" {
		return s
	}

	return strings.ToLower(s[:1]) + s[1:]
}

// loadServerData resolves posArgs (or auto-discovers under schemaDir) into
// schema-driven wiring data for serverTemplate. When no schema files are
// found at all (neither passed explicitly nor discovered), it returns a
// zero-Modules serverData rather than an error: `zever generate server` is
// also how a brand-new project scaffolds its entrypoint before any schema
// exists.
func loadServerData(modulePath, outDir, schemaDir string, posArgs []string, rateLimitEnabled bool) (serverData, error) {
	data := serverData{ModulePath: modulePath, RateLimitEnabled: rateLimitEnabled}

	paths, err := resolveInputFiles(posArgs)
	if err != nil {
		if errors.Is(err, errNoInputFiles) {
			return data, nil
		}

		return data, err
	}

	if len(posArgs) == 0 {
		reportAutoDiscovery(paths)
	}

	files, err := loadFiles(paths)
	if err != nil {
		return data, err
	}

	result, diags := compile.WithSchemaDir(files, schemaDir)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	if diags.HasErrors() {
		return data, fmt.Errorf("zever generate server: %d file(s) failed to compile", len(files))
	}

	for _, m := range result.Schema.Modules {
		if len(m.Services) == 0 {
			continue
		}

		pkg, dir := gogen.ModuleNaming(m)

		md := serverModuleData{
			Alias:          "gen" + pkg,
			ImportPath:     genImportPath(modulePath, outDir, dir),
			NeedsAuthz:     gogen.ModuleNeedsAuthz(m),
			ImplAlias:      pkg + "impl",
			ImplImportPath: modulePath + "/" + serverStubRoot + "/" + dir,
			ImplDir:        serverStubRoot + "/" + dir,
		}

		for _, svc := range m.Services {
			md.Services = append(md.Services, serverServiceData{
				Name:        svc.Name,
				ImplVar:     lowerFirst(svc.Name) + "Impl",
				Constructor: "New" + svc.Name + "Impl",
				ir:          svc,
			})
		}

		data.Modules = append(data.Modules, md)
	}

	return data, nil
}

// writeServiceStubs writes GenerateServiceStub's output for every service in
// data that has no stub file yet, returning the paths actually written.
// Skipped silently (no error, no overwrite) when the file already exists --
// see GenerateServiceStub's own doc comment on why a stub is never part of
// an always-regenerated output set.
func writeServiceStubs(tag string, data serverData) ([]string, error) {
	var written []string

	for _, mod := range data.Modules {
		for _, svc := range mod.Services {
			path := filepath.Join(mod.ImplDir, naming.SnakeCase(svc.Name)+".go")

			content, err := gogen.GenerateServiceStub(mod.ImplAlias, mod.Alias, mod.ImportPath, svc.ir)
			if err != nil {
				return written, fmt.Errorf("%s: generate stub for %s: %w", tag, svc.Name, err)
			}

			ok, err := writeStubIfMissing(tag, path, content)
			if err != nil {
				return written, err
			}

			if ok {
				written = append(written, path)
			}
		}
	}

	return written, nil
}

// writeStubIfMissing writes content to path only when nothing is there yet,
// returning written=false (no error) when a file already exists at path.
// This is the skip-if-exists contract every hand-implementable stub this
// package scaffolds shares (service, job, seed implementations): unlike
// writeScaffold's always-regenerated files, a stub is meant to be filled in
// by hand and must never be silently clobbered by a later regeneration.
func writeStubIfMissing(tag, path string, content []byte) (written bool, err error) {
	if _, statErr := os.Stat(path); statErr == nil {
		return false, nil
	} else if !os.IsNotExist(statErr) {
		return false, fmt.Errorf("%s: stat %q: %w", tag, path, statErr)
	}

	if err := writeScaffold(tag, path, content, false); err != nil {
		return false, err
	}

	return true, nil
}

// appTemplate mirrors examples/todo/internal/app/app.go: the one place a
// project selects its adapters and builds its container. Rendered via
// renderAppContent, never directly -- see appTemplateData.
const appTemplate = `// Package app builds the config and container shared by this project's
// binaries.
//
// The blank imports below are the whole of the "wiring" a zever app has to
// do: a battery's registry only learns about an adapter once that adapter
// package's init() has run, so an app imports exactly the adapters it
// actually uses and nothing else. This list was picked at scaffold time
// (` + "`zever new`" + `'s battery picker, or the fixed set every generated
// entrypoint needs when there was no prior ` + "`zever new`" + `) -- add a battery
// later by giving it an adapter in zever.yaml and adding its one blank
// import below by hand; this file is scaffolded once and never regenerated.
package app

import (
	"fmt"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"

	// Blank imports register the adapters this app selects; see the package
	// comment.
{{range .Imports}}	_ {{printf "%q" .}}
{{end}})

// DefaultDBPath is the sqlite file used when nothing else supplies one. It is
// relative to the process working directory.
const DefaultDBPath = "data/app.db"

// Config returns the resolved configuration: zever's zero-infra defaults,
// overlaid by any zever.yaml in the working directory and by ZEVER_*
// environment variables, with a sqlite path filled in when nothing supplied
// one.
func Config() (*config.Config, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("[app] load config: %w", err)
	}

	dbc := cfg.DB
	if dbc.Options.Path == "" {
		dbc.Options.Path = DefaultDBPath
	}

	cfg.DB = dbc

	return cfg, nil
}

// New builds the container every binary in this project uses. Nothing is
// opened until a battery is first requested.
func New() (*container.Container, error) {
	cfg, err := Config()
	if err != nil {
		return nil, err
	}

	return container.New(cfg), nil
}
`

// serverTemplate is a direct transcription of
// examples/todo/cmd/server/main.go's structure, parameterized on the target
// project's module path. Only the project-specific route/service
// registration is replaced with a TODO, because routes and gRPC service
// registration are hand-wired Go code rather than something the IR can
// produce standalone.
//
// This is the scaffold that makes "gRPC by default" real: it starts BOTH
// an HTTP server (-addr, default :8080) and a gRPC server (-grpc-addr,
// default :9090) from the SAME container.Container, and wires the SAME
// per-module policy map (each gogen-rendered module package's own
// GRPCPolicies(), merged) into gRPC's single grpc.UnaryInterceptor and
// into every HTTP route registration's auth/permission parameters -- one
// enforcement decision (authz.Authorize) reachable through either
// transport, never two parallel implementations. See container.Container's
// GRPC method doc comment for why the interceptor has to be assembled here
// rather than inside the schema-agnostic container package itself.
const serverTemplate = `// Command server runs this project's HTTP+gRPC API.
//
// Scaffolded by ` + "`zever generate server`" + `. It is deliberately thin: parse
// flags, build the container, register HTTP routes and gRPC services,
// serve both, shut down both. Everything with behaviour worth testing
// belongs in a package of its own.
package main

import (
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/zenta-dev/zever/authz"
	"github.com/zenta-dev/zever/middleware"

	"{{.ModulePath}}/internal/app"
{{range .Modules}}	{{.Alias}} "{{.ImportPath}}"
	{{.ImplAlias}} "{{.ImplImportPath}}"
{{end}})

const (
	shutdownGrace     = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	readyTimeout      = 3 * time.Second

	// gRPC hardening defaults: bound idle connections, detect dead peers,
	// and cap message size so one client can't exhaust server memory.
	grpcMaxConnectionIdle = 5 * time.Minute
	grpcKeepaliveTime     = 2 * time.Minute
	grpcKeepaliveTimeout  = 20 * time.Second
	grpcMaxMessageBytes   = 4 << 20 // 4 MiB, grpc-go's own default made explicit.
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP address to listen on")
	grpcAddr := flag.String("grpc-addr", ":9090", "gRPC address to listen on")

	flag.Parse()

	if err := run(*addr, *grpcAddr); err != nil {
		// The log battery is not necessarily available this early, so the
		// last-resort failure path writes to stderr directly.
		_, _ = os.Stderr.WriteString("server: " + err.Error() + "\n")

		os.Exit(1)
	}
}

func run(addr, grpcAddr string) error {
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

	authInst, err := c.Auth()
	if err != nil {
		return err
	}

	permInst, err := c.Permission()
	if err != nil {
		return err
	}

	obs, err := c.Observability()
	if err != nil {
		return err
	}
{{if .RateLimitEnabled}}
	limiter, err := c.RateLimit()
	if err != nil {
		return err
	}
{{end}}
	// One interceptor, attached once below, must cover every service this
	// process registers on grpcServer -- so every generated module
	// package's own GRPCPolicies() is merged into one map here first.
	policies := map[string]authz.Policy{}
{{range .Modules}}
	for k, v := range {{.Alias}}.GRPCPolicies() {
		policies[k] = v
	}
{{end}}
{{if not .Modules}}
	// TODO: pass .zen schema files to 'zever generate server' to wire
	// GRPCPolicies()/RegisterModule for every declared module automatically,
	// e.g.: zever generate server schema/*.zen
{{end}}
	grpcServer, err := c.GRPC(
		grpc.ChainUnaryInterceptor(
			middleware.RecoverUnaryServerInterceptor(logger),
			middleware.TracingUnaryServerInterceptor(obs),
{{if .RateLimitEnabled}}			middleware.RateLimitUnaryServerInterceptor(limiter, middleware.PeerAddrKey),
{{end}}			authz.UnaryServerInterceptor(authInst, permInst, policies),
		),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: grpcMaxConnectionIdle,
			Time:              grpcKeepaliveTime,
			Timeout:           grpcKeepaliveTimeout,
		}),
		grpc.MaxRecvMsgSize(grpcMaxMessageBytes),
		grpc.MaxSendMsgSize(grpcMaxMessageBytes),
	)
	if err != nil {
		return err
	}

	r, err := c.Router()
	if err != nil {
		return err
	}

	r.Use(middleware.Recover(logger), middleware.RequestLogger(logger), middleware.Tracing(obs))
{{if .RateLimitEnabled}}
	r.Use(middleware.RateLimit(limiter, middleware.RemoteAddrKey))
{{end}}
	database, err := c.DB()
	if err != nil {
		return err
	}

	r.Handle("GET", "/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Handle("GET", "/readyz", func(w http.ResponseWriter, req *http.Request) {
		pingCtx, cancel := context.WithTimeout(req.Context(), readyTimeout)
		defer cancel()

		if err := database.Ping(pingCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		w.WriteHeader(http.StatusOK)
	})
{{range .Modules}}{{$mod := .}}
	{{range .Services}}{{.ImplVar}} := {{$mod.ImplAlias}}.{{.Constructor}}()
	{{end}}
	{{if $mod.NeedsAuthz}}{{$mod.Alias}}.RegisterModule(r, grpcServer, authInst, permInst, {{$mod.Alias}}.ModuleImpls{
		{{range .Services}}{{.Name}}: {{.ImplVar}},
		{{end}}
	}){{else}}{{$mod.Alias}}.RegisterModule(r, grpcServer, {{$mod.Alias}}.ModuleImpls{
		{{range .Services}}{{.Name}}: {{.ImplVar}},
		{{end}}
	}){{end}}
{{end}}
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	errs := make(chan error, 2)

	go func() {
		logger.Info().Str("addr", addr).Msg("http server listening")

		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err

			return
		}

		errs <- nil
	}()

	go func() {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			errs <- err

			return
		}

		logger.Info().Str("addr", grpcAddr).Msg("grpc server listening")

		if err := grpcServer.Serve(lis); err != nil {
			errs <- err

			return
		}

		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		logger.Info().Msg("server shutting down")

		grpcServer.GracefulStop()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()

		return httpServer.Shutdown(shutdownCtx)
	}
}
`
