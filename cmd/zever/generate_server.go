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
	"strconv"
	"strings"
	"text/template"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/dsl/backend/gogen"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/naming"
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

// coreBatteries is the fixed, always-scaffolded battery set: the minimal
// floor every generated entrypoint needs.
//
//   - log (slog): stdlib logger; every entrypoint resolves c.Log() first.
//   - router (stdhttp): net/http based; fiber is the heavy alternative.
//
// Both are adapter modules with a Register() wiring their battery registry
// (see adapterBinding): the container wires nothing itself. Anything beyond
// the floor is added via `zever new`'s battery picker (or `zever add`, or
// with service env + app.go).
var coreBatteries = []string{
	"log", "router",
}

// batterySelection is one battery+adapter pair, the single shape both
// `zever new`'s battery picker (new.go) and appTemplate render from, so the
// scaffolded service set and app.go's blank imports can never drift apart.
type batterySelection struct {
	Battery string
	Adapter string
}

// adapterDirName maps a battery selection to its adapter module directory
// ("<battery>/<adapter>"), the path segment batteryImportPath builds on.
// It is mechanical across every pair with five exceptions: the password
// adapter string is PHC-canonical "argon2id" while its package directory
// is "argon2", the resilience "memory" adapter is served by the
// "resilience/inproc" package directory, and the search/vectorstore/workflow
// legacy DB-backed adapter names ("postgres", "pgvector", "sqlite") are
// served by consolidated "db" package directories (empty DSN selects the
// embedded sqlite backend) since adapters/search/sqlite and
// adapters/vectorstore/sqlite were folded into them. The canonical "db"
// name is mechanical (search/db and friends).
func adapterDirName(b batterySelection) string {
	if b.Battery == "password" && b.Adapter == "argon2id" {
		return "password/argon2"
	}

	if b.Battery == "resilience" && b.Adapter == "memory" {
		return "resilience/inproc"
	}

	if b.Battery == "search" && (b.Adapter == "db" || b.Adapter == "sqlite" || b.Adapter == "postgres") {
		return "search/db"
	}

	if b.Battery == "vectorstore" && (b.Adapter == "db" || b.Adapter == "sqlite" || b.Adapter == "pgvector") {
		return "vectorstore/db"
	}

	if b.Battery == "workflow" && (b.Adapter == "db" || b.Adapter == "postgres") {
		return "workflow/db"
	}

	return b.Battery + "/" + b.Adapter
}

// batteryImportPath returns b's adapter package import path. Mechanical
// across every battery/adapter pair: every adapter lives at
// "adapters/<battery>/<adapter>" (e.g. adapters/cache/memory,
// adapters/db/sqlite), matching config.Default()'s own battery/adapter
// naming exactly (modulo adapterDirName's password/db exceptions).
func batteryImportPath(b batterySelection) string {
	return "github.com/zenta-dev/zever/adapters/" + adapterDirName(b)
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
		"log":    cfg.Log.Adapter,
		"router": cfg.Router.Adapter,
	}

	sel := make([]batterySelection, 0, len(coreBatteries))

	for _, name := range coreBatteries {
		sel = append(sel, batterySelection{Battery: name, Adapter: byName[name]})
	}

	return sel
}

// appTemplateData is appTemplate's render input: AdapterImports are the
// adapter modules' named imports (alias + path, e.g. logslog +
// adapters/log/slog) the selection needs, CoreImports the core packages
// the mapped adapter factories' signatures name, RefVars the per-instance
// Ref declarations, and Providers the qualified container.Provide entries
// -- all sorted for deterministic output.
type appTemplateData struct {
	AdapterImports []moduleImport
	CoreImports    []string
	RefVars        []string
	Providers      []string
}

// moduleImport is one adapter module import in the generated app.go:
// Alias is the local name the Provide factories are qualified with
// (battery + adapter, e.g. logslog), Path the adapter package import path.
type moduleImport struct {
	Alias string
	Path  string
}

// adapterBinding maps one battery selection to its adapter module: the
// alias app.go imports it under and the adapter package import path. Every
// battery/adapter pair binds: every adapter (stdlib-only included) is
// a module with a Register() wiring their battery registry, and the
// container wires nothing itself. Aliases are battery + adapter with no
// separator; every pair is unique and collides with neither stdlib
// names nor facade package names.
func adapterBinding(b batterySelection) (alias, path string) {
	alias = b.Battery + b.Adapter

	return alias, batteryImportPath(b)
}

// renderAppContent renders appTemplate for exactly the given battery
// selection -- the one place that turns a []batterySelection into app.go's
// content, so `zever generate server` (floor selection) and any future
// caller can never format the import block differently. The output wires
// exactly the selection (floor + user picks, nothing more): one Ref var
// plus one container.Provide entry per selection, the named adapter
// imports, and the core packages the mapped factories' signatures name,
// all sorted deterministically. The Provide entries are the whole of the
// wiring: the container validates the graph and resolves each instance
// lazily on first use.
func renderAppContent(tag string, selections []batterySelection) ([]byte, error) {
	sorted := append([]batterySelection(nil), selections...)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Battery != sorted[j].Battery {
			return sorted[i].Battery < sorted[j].Battery
		}

		return sorted[i].Adapter < sorted[j].Adapter
	})

	adapterImports := make([]moduleImport, 0, len(sorted))
	coreSeen := map[string]bool{}
	coreImports := []string{}
	refVars := make([]string, 0, len(sorted))
	providers := make([]string, 0, len(sorted))

	for _, s := range sorted {
		alias, path := adapterBinding(s)
		adapterImports = append(adapterImports, moduleImport{Alias: alias, Path: path})

		if core := coreModulePath(s.Battery); !coreSeen[core] {
			coreSeen[core] = true
			coreImports = append(coreImports, core)
		}

		varName := refVarName(s.Battery, "default")
		refVars = append(refVars, fmt.Sprintf("// %s names the default %s instance (%s adapter).\nvar %s = %s.DefaultRef()", varName, s.Battery, s.Adapter, varName, s.Battery))

		factory, err := factoryExpr(s, alias, "\t\t")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", tag, err)
		}

		providers = append(providers, "container.Provide("+varName+", "+factory+")")
	}

	sort.Slice(adapterImports, func(i, j int) bool { return adapterImports[i].Path < adapterImports[j].Path })
	sort.Strings(coreImports)

	return renderGoFile(tag, "app.go", appTemplate, appTemplateData{
		AdapterImports: adapterImports,
		CoreImports:    coreImports,
		RefVars:        refVars,
		Providers:      providers,
	})
}

// ensureAppPackage scaffolds internal/app/app.go when the target project has
// no wiring of its own. `zever new` scaffolds the same wiring under
// internal/app/wiring.go (its Ref vars and Providers share this file's
// names), so either file existing means the project owns its adapter
// selection and this create-if-missing step must stay out of the way.
// Called with no battery-selection context (e.g. `zever generate server`
// run without a prior `zever new`), so it renders with just
// coreBatterySelections -- the floor (log, router), both adapter modules
// with Provide entries, not a developer's fuller pick.
func ensureAppPackage(tag string) (created bool, err error) {
	path := filepath.Join("internal", "app", "app.go")

	if _, statErr := os.Stat(path); statErr == nil {
		return false, nil
	} else if !os.IsNotExist(statErr) {
		return false, fmt.Errorf("%s: stat %q: %w", tag, path, statErr)
	}

	if _, statErr := os.Stat(filepath.Join("internal", "app", "wiring.go")); statErr == nil {
		return false, nil
	} else if !os.IsNotExist(statErr) {
		return false, fmt.Errorf("%s: stat %q: %w", tag, filepath.Join("internal", "app", "wiring.go"), statErr)
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

// generateServerDryRun validates and renders the server entrypoint without
// writing anything, reporting the planned paths.
func generateServerDryRun(cfg GenerateServerConfig) (GenerateServerResult, error) {
	const tag = "zever generate server"

	var res GenerateServerResult

	data, err := loadServerData(cfg.ModulePath, cfg.OutDir, cfg.SchemaDir, cfg.SchemaFiles, cfg.RateLimitEnabled, cfg.ReflectionEnabled)
	if err != nil {
		return res, err
	}

	if _, err := renderGoFile(tag, "server main.go", serverTemplate, data); err != nil {
		return res, err
	}

	if _, err := renderModulesContent(tag, data); err != nil {
		return res, err
	}

	res.Modules = len(data.Modules)
	res.Entrypoint = filepath.Join(cfg.ServerEntry, "main.go")
	res.ModulesFile = filepath.Join("internal", "app", modulesFileName)

	if stdout := cfg.Stdout; stdout != nil {
		printGenerateDryRun(stdout, res.Entrypoint)
		printGenerateDryRun(stdout, res.ModulesFile)

		if _, statErr := os.Stat(filepath.Join("internal", "app", "app.go")); os.IsNotExist(statErr) {
			if _, statErr := os.Stat(filepath.Join("internal", "app", "wiring.go")); os.IsNotExist(statErr) {
				_, _ = fmt.Fprintln(stdout, dim("would write ")+cyan(filepath.Join("internal", "app", "app.go")))
			}
		}

		if len(data.Modules) > 0 {
			_, _ = fmt.Fprintln(stdout, dim(fmt.Sprintf("would stub services for %d module(s)", len(data.Modules))))
		}
	}

	return res, nil
}

const serverUsageBody = `Scaffolds a thin HTTP+gRPC server entrypoint at <server_entry>/main.go
(default cmd/server): a flag-parsing wrapper over the serve library (the
same shape "zever new" writes) that passes the project's Providers and
the schema modules to serve.Main. All boot behavior -- listeners, TLS,
health, readiness, rate limiting, reflection, pprof, doctor -- lives in
serve; project wiring lives in internal/app.

Also scaffolds internal/app/app.go when the project does not have one yet
(adapter selection, stub-once) and refreshes internal/app/modules.go on every
run (one serve.Module wrapper per schema module, delegating to the generated
RegisterModule wiring). Pass .zen schema files (or rely on auto-discovery
under the project's schema dir) and every declared module's HTTP routes and
gRPC services register automatically. For every service with no stub
implementation yet, a compiling (Unimplemented) starting-point stub is also
written under internal/service/<module>/ -- never overwritten once it
exists (HARD RULE R1: re-running after you've started implementing is always
safe; new schema entities create only new stubs, removed ones only warn).
Requires the schema to have already been compiled with the gogen backend
(zever compile --backend gogen) into the same --out directory.`

func printServerUsage(fs *flag.FlagSet) {
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

	printFlagDefaults(fs)
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
	// ReflectionEnabled gates the gRPC reflection wiring (see
	// reflectionEnabled); callers obtain it from the environment, not flags.
	ReflectionEnabled bool
	Stdout            io.Writer
	Stderr            io.Writer
	// DryRun prints the planned writes without touching the filesystem.
	DryRun bool
}

// GenerateServerResult names everything GenerateServer wrote.
type GenerateServerResult struct {
	Entrypoint string
	// ModulesFile is the refreshed internal/app wiring file, if written.
	ModulesFile string
	AppCreated  bool
	Stubs       []string
	Modules     int
}

// GenerateServer renders the server entrypoint (and service stubs) from
// resolved inputs and writes them to disk. It performs no flag parsing and
// no prompting.
func GenerateServer(cfg GenerateServerConfig) (GenerateServerResult, error) {
	const tag = "zever generate server"

	var res GenerateServerResult

	if cfg.DryRun {
		return generateServerDryRun(cfg)
	}

	appCreated, err := ensureAppPackage(tag)
	if err != nil {
		return res, err
	}

	res.AppCreated = appCreated

	data, err := loadServerData(cfg.ModulePath, cfg.OutDir, cfg.SchemaDir, cfg.SchemaFiles, cfg.RateLimitEnabled, cfg.ReflectionEnabled)
	if err != nil {
		return res, err
	}

	res.Modules = len(data.Modules)

	mainContent, err := renderGoFile(tag, "server main.go", serverTemplate, data)
	if err != nil {
		return res, err
	}

	mainPath := filepath.Join(cfg.ServerEntry, "main.go")

	if writeErr := writeScaffold(tag, mainPath, mainContent, cfg.Force); writeErr != nil {
		return res, writeErr
	}

	res.Entrypoint = mainPath

	modulesContent, err := renderModulesContent(tag, data)
	if err != nil {
		return res, err
	}

	modulesPath := filepath.Join("internal", "app", modulesFileName)

	if writeErr := writeScaffold(tag, modulesPath, modulesContent, cfg.Force); writeErr != nil {
		return res, writeErr
	}

	res.ModulesFile = modulesPath

	stubsWritten, err := writeServiceStubs(tag, data)
	if err != nil {
		return res, err
	}

	res.Stubs = stubsWritten

	warnRemovedStubs(tag, data, cfg.Stderr)

	if stdout := cfg.Stdout; stdout != nil {
		if appCreated {
			_, _ = fmt.Fprintf(stdout, "%s %s\n", successMark(), success("scaffolded ")+cyan(filepath.Join("internal", "app", "app.go")))
		}

		_, _ = fmt.Fprintf(stdout, "%s %s %s\n", successMark(), success("scaffolded server entrypoint at"), cyan(mainPath))
		_, _ = fmt.Fprintf(stdout, "%s %s %s\n", successMark(), success("refreshed module wiring at"), cyan(modulesPath))

		for _, p := range stubsWritten {
			_, _ = fmt.Fprintf(stdout, "%s %s %s\n", successMark(), success("scaffolded service stub at"), cyan(p))
		}
	}

	if stderr := cfg.Stderr; stderr != nil && shouldShowHint() {
		if len(data.Modules) == 0 {
			_, _ = fmt.Fprintln(stderr, formatHint("pass .zen schema files to wire modules automatically, e.g. zever generate server schema/*.zen"))
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
	dryRun := fs.Bool("dry-run", false, "print the planned writes without writing files")
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
	reflectionEnabled := reflectionEnabled()

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
		ModulePath:        modulePath,
		OutDir:            *outDir,
		SchemaDir:         project.SchemaDir,
		ServerEntry:       project.ServerEntry,
		SchemaFiles:       posArgs,
		Force:             forceVal,
		RateLimitEnabled:  rateLimitEnabled,
		ReflectionEnabled: reflectionEnabled,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		DryRun:            *dryRun,
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
	// ModuleName is the schema module name (gogen's dir name when unnamed),
	// reported by the generated serve.Module's Name.
	ModuleName string
	// TypeName is the Pascal-case wrapper type name, e.g. "AppModule".
	TypeName string
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

// serverData is the full data the server templates render against.
type serverData struct {
	ModulePath string
	Modules    []serverModuleData
	// Stamp is the provenance header prepended to every refreshed file
	// (see scaffoldStampLine).
	Stamp string
	// SchemaHash fingerprints the schema revision the wiring was rendered
	// from; refreshed files stamp it so a reader can tell which revision
	// produced them.
	SchemaHash string
	// AnyNeedsAuthz is true when at least one module's RegisterModule takes
	// auth/permission params, gating those imports in modules.go.
	AnyNeedsAuthz bool
	// RateLimitEnabled is true only when the resolved service env
	// configures a real rate/burst for the "ratelimit" battery
	// -- see rateLimitConfigured's doc comment for why this can't simply be
	// "always resolve c.RateLimit()".
	RateLimitEnabled bool
	// Reflection is true when gRPC server reflection should be wired into
	// the generated server: off by default (reflection exposes every service
	// schema to any network client), on only via GRPC_REFLECTION=true or a
	// dev ZEVER_ENV -- see reflectionEnabled.
	Reflection bool
}

// devEnv reports whether ZEVER_ENV marks a non-production environment
// (trimmed, case-insensitive) via the shared crypto.IsDevEnv. Every other
// value -- including production and an unset or empty ZEVER_ENV -- is non-dev.
func devEnv() bool {
	return crypto.IsDevEnv(os.Getenv("ZEVER_ENV"))
}

// reflectionEnabled reports whether the generated server should register
// gRPC server reflection. Secure-by-default: off unless the operator sets
// GRPC_REFLECTION=true (an explicit false always wins, even in dev) or the
// process runs in a dev ZEVER_ENV, where reflection is a genuine local
// convenience (grpcurl against the dev server).
func reflectionEnabled() bool {
	if v, ok := os.LookupEnv("GRPC_REFLECTION"); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b
		}
	}

	return devEnv()
}

// rateLimitConfigured reports whether the calling project has configured a
// real rate limit for the "ratelimit" battery. Unlike most batteries,
// ratelimit/memory's zero-infra adapter still requires an explicit
// rate/burst and errors without one, so unconditionally resolving
// c.RateLimit() in the generated main.go would break every project that
// hasn't configured a limit. Rate-limit wiring is instead emitted only when
// the config says a limit is actually set -- add RATELIMIT_RATE/RATELIMIT_BURST
// to the env and re-run `zever generate server --force` to turn it on,
// no other wiring needed.
//
// Adaptation note: zever's config is typed, so this reads
// cfg.RateLimit.Options.Rate directly instead of zen-go's
// cfg.Get("ratelimit").Options["rate"] JSON-round-tripped map lookup.
// An unreadable config falls back to config.Default() instead of failing
// generation -- the real error still surfaces at serve time. zever's
// defaults ship a real limit (rate 10, burst 20), so the fallback wires
// the limiter on.
func rateLimitConfigured() bool {
	env, err := config.FromEnv()
	if err != nil {
		return config.Default().RateLimit.Options.Rate > 0
	}
	cfg, err := config.Resolve(env)
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
func loadServerData(modulePath, outDir, schemaDir string, posArgs []string, rateLimitEnabled, reflectionEnabled bool) (serverData, error) {
	data := serverData{ModulePath: modulePath, RateLimitEnabled: rateLimitEnabled, Reflection: reflectionEnabled}
	data.SchemaHash = schemaHashOf(nil)
	data.Stamp = scaffoldStampLine(data.SchemaHash)

	paths, err := resolveInputFiles(posArgs)
	if err != nil {
		if errors.Is(err, errNoInputFiles) {
			return data, nil
		}

		return data, fmt.Errorf("zever generate server: %w", err)
	}

	if len(posArgs) == 0 {
		reportAutoDiscovery(paths)
	}

	files, err := loadFiles(paths)
	if err != nil {
		return data, fmt.Errorf("zever generate server: %w", err)
	}

	data.SchemaHash = schemaHashOf(files)
	data.Stamp = scaffoldStampLine(data.SchemaHash)

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
		name := m.Name
		if name == "" {
			name = dir
		}

		md := serverModuleData{
			Alias:          "gen" + pkg,
			ImportPath:     genImportPath(modulePath, outDir, dir),
			ModuleName:     name,
			TypeName:       naming.PascalCase(pkg) + "Module",
			NeedsAuthz:     gogen.ModuleNeedsAuthz(m),
			ImplAlias:      pkg + "impl",
			ImplImportPath: modulePath + "/" + serverStubRoot + "/" + dir,
			ImplDir:        serverStubRoot + "/" + dir,
		}

		if md.NeedsAuthz {
			data.AnyNeedsAuthz = true
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

// warnRemovedStubs warns about hand-written stub files that no longer match
// any service in the schema: HARD RULE R1 says an existing stub is left
// byte-for-byte and a removed entity only warns, never deletes. A nil
// stderr silences the warning.
func warnRemovedStubs(tag string, data serverData, stderr io.Writer) {
	if stderr == nil {
		return
	}

	for _, mod := range data.Modules {
		keep := make(map[string]bool, len(mod.Services))
		for _, svc := range mod.Services {
			keep[filepath.Join(mod.ImplDir, naming.SnakeCase(svc.Name)+".go")] = true
		}

		entries, err := os.ReadDir(mod.ImplDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}

			path := filepath.Join(mod.ImplDir, entry.Name())
			if !keep[path] {
				_, _ = fmt.Fprintf(stderr, "%s: %s no longer matches a schema service; left in place (delete by hand if intended)\n", tag, path)
			}
		}
	}
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
//
// Adapter packages expose constructors but never self-register, so wiring
// is explicit: the named imports below pin exactly the adapters this app
// uses, and each has a matching Register call in New wiring its battery
// registry. This list was picked at scaffold time (`zever new`'s battery
// picker, or the floor (log, router) when there was no prior `zever new`)
// -- add a battery later with `zever add <battery>` (or with service env plus
// one import and Register call here); this file is
// scaffolded once and never regenerated.
const appTemplate = `// Package app builds the config and container shared by this project's
// binaries.
//
// The Ref vars below name each wired instance; Providers builds them from
// the named adapter constructors. This list was picked at scaffold time
// (` + "`zever generate server`" + `'s floor (log, router) when there was no prior ` + "`zever new`" + `,
// or the project's own picks once it has one) -- add a battery later with
// ` + "`zever add <battery>`" + `: it only inserts Ref vars and Provider entries
// here and never rewrites user code. Tune an instance with
// container.WithOptions entries in Providers.
package app

import (
	"fmt"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
{{range .CoreImports}}	"{{.}}"
{{end}}{{range .AdapterImports}}	{{.Alias}} {{printf "%q" .Path}}
{{end}})

// DefaultDBPath is the sqlite file used when nothing else supplies one. It is
// relative to the process working directory.
const DefaultDBPath = "data/app.db"

// Config returns the resolved configuration: zever's zero-infra defaults,
// overlaid by .env/.env.local discovery and <SERVICE>_* environment
// variables, with a sqlite path filled in when nothing supplied
// one.
func Config() (*config.Config, error) {
	env, err := config.FromEnv()
	if err != nil {
		return nil, fmt.Errorf("app: load env: %w", err)
	}

	cfg, err := config.Resolve(env)
	if err != nil {
		return nil, fmt.Errorf("app: load config: %w", err)
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

	return container.New(cfg, Providers()...), nil
}

{{range .RefVars}}
{{.}}{{end}}
// Providers returns every wired service instance, in declaration order.
// The container validates the graph (duplicate refs, missing deps,
// cycles) and resolves each instance lazily on first use.
func Providers() []container.Provider {
	return []container.Provider{
{{range .Providers}}		{{.}},
{{end}}	}
}
`

// serverTemplate renders cmd/server/main.go: the same flag-parsing wrapper
// `zever new` scaffolds (see newServerMainTemplate), so the two commands can
// never fight over the file's shape. It parses the listen flags the launcher
// passes through, then hands the typed Providers and the schema modules to
// serve.Main. serve returns the error; main owns the exit path (stderr +
// non-zero).
//
// -addr overrides the HTTP port (":8080", "host:port" or bare "8080"; only
// the port is used, serve binds all interfaces and warns on plaintext
// non-loopback). Empty falls back to serve's own PORT/default. -grpc-addr
// enables gRPC only when auth+permission are wired (see grpcWired):
// otherwise a one-line notice explains the skip, so `zever dev -grpc-addr
// ...` stays green on scaffolds that never wired them.
const serverTemplate = `// Command server runs this project's HTTP+gRPC API.
//
// Scaffolded by ` + "`zever generate server`" + `. Deliberately thin:
// serve.Main owns boot, signal handling and graceful shutdown; everything
// with behaviour worth testing belongs in a package of its own.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/serve"

	"{{.ModulePath}}/internal/app"
)

func main() {
	addr := flag.String("addr", "", "HTTP listen address (:port or host:port; empty uses PORT, default :8080)")
	grpcAddr := flag.String("grpc-addr", "", "gRPC listen address (needs auth+permission wired; empty disables)")
	flag.Parse()

	opts := []serve.Option{}

	if port, err := serverPort(*addr); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	} else if port != "" {
		opts = append(opts, serve.WithPort(port))
	}

	if *grpcAddr != "" {
		if !grpcWired() {
			fmt.Fprintln(os.Stderr, "server: -grpc-addr skipped (wire auth+permission via 'zever add auth permission' to enable gRPC)")
		} else {
			opts = append(opts, serve.WithGRPCAddr(*grpcAddr))
		}
	}

	modules := func({{if .Modules}}c{{else}}_{{end}} *container.Container) []serve.Module {
		return []serve.Module{
{{range .Modules}}			app.New{{.TypeName}}(c),
{{end}}		}
	}

	if err := serve.Main(app.Providers(), modules, opts...); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}

// grpcWired reports whether the project's Providers wire the auth and
// permission instances gRPC serving requires. It builds a throwaway
// container (nothing resolves until first use) and inspects the provider
// labels, mirroring what serve itself demands in setupGRPC.
func grpcWired() bool {
	c := container.New(nil, app.Providers()...)

	auth, permission := false, false

	for _, label := range c.Wired() {
		switch label {
		case "auth", "auth/default":
			auth = true
		case "permission", "permission/default":
			permission = true
		}
	}

	return auth && permission
}

// serverPort extracts the port from an -addr value (":8080",
// "127.0.0.1:8080" or bare "8080"), failing closed on garbage. The empty
// address means "no override".
func serverPort(addr string) (string, error) {
	if addr == "" {
		return "", nil
	}

	if _, port, err := net.SplitHostPort(addr); err == nil {
		if port == "" {
			return "", fmt.Errorf("-addr %q names no port", addr)
		}

		return port, nil
	}

	if _, err := strconv.Atoi(addr); err == nil {
		return addr, nil
	}

	return "", fmt.Errorf("-addr %q wants :port or host:port", addr)
}
`

// modulesFileName is the refreshed wiring file under internal/app: the
// serve.Module wrappers the generated main mounts, regenerated on every
// generate from the current schema. Service implementations are the opposite
// contract (see writeServiceStubs): stub-once, never overwritten.
const modulesFileName = "modules.go"

// scaffoldStampLine is the provenance header for refreshed scaffold files:
// the CLI release that rendered them plus the schema hash they were rendered
// from. Scaffold files are compared byte-for-byte, so the version stays
// embedded (unlike the stampOnly-tolerant compile outputs).
func scaffoldStampLine(hash string) string {
	return fmt.Sprintf("// Code generated by zever generate server v%s; DO NOT EDIT.\n// schema-hash: sha256:%s", buildVersion, hash)
}

// modulesTemplate renders internal/app/modules.go: one serve.Module wrapper
// per schema module, from which main builds the module list it hands to
// serve.Main. It is refreshed on every generate (force-written): hand edits
// belong in internal/service, never here. Every module implements
// serve.GRPCModule by delegating to its generated gogen package's
// GRPCPolicies, so serve's fail-closed interceptor covers every service with
// no per-module merge code in main.
const modulesTemplate = `{{.Stamp}}
// Package app wires the project's schema-declared modules into the serve
// library: one serve.Module wrapper per module that the generated main
// mounts through serve.Main. This file is refreshed on every generate from
// the current schema; put hand-written logic in internal/service, never
// here.
package app
{{if .Modules}}
import (
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/serve"
{{if .AnyNeedsAuthz}}	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/permission"
{{end}}{{range .Modules}}	{{.Alias}} "{{.ImportPath}}"
	{{.ImplAlias}} "{{.ImplImportPath}}"
{{end}})
{{end}}
{{range .Modules}}{{$mod := .}}
// {{$mod.TypeName}} mounts the {{$mod.ModuleName}} module's HTTP routes and
// gRPC services.
type {{$mod.TypeName}} struct {
	c *container.Container
}

// New{{$mod.TypeName}} builds the {{$mod.ModuleName}} module mount.
func New{{$mod.TypeName}}(c *container.Container) *{{$mod.TypeName}} {
	return &{{$mod.TypeName}}{c: c}
}

var _ serve.GRPCModule = (*{{$mod.TypeName}})(nil)

// Name identifies the module in serve errors and logs.
func (m *{{$mod.TypeName}}) Name() string {
	return {{printf "%q" $mod.ModuleName}}
}

// GRPCPolicies delegates to the generated package's policy map: every
// operation the schema exposes over gRPC carries an entry, so serve's
// fail-closed interceptor denies anything unlisted.
func (m *{{$mod.TypeName}}) GRPCPolicies() map[string]authz.Policy {
	return {{$mod.Alias}}.GRPCPolicies()
}

// Mount registers the module's HTTP routes and gRPC services. Auth and
// permission resolve lazily so modules without guarded operations never
// require those batteries.
func (m *{{$mod.TypeName}}) Mount(r router.Router, _ *container.Container) error {
	c := m.c
{{if $mod.NeedsAuthz}}	authInst, err := c.Auth()
	if err != nil {
		return err
	}

	permInst, err := c.Permission()
	if err != nil {
		return err
	}

{{end}}	grpcServer, err := c.GRPC()
	if err != nil {
		return err
	}
{{range $mod.Services}}	{{.ImplVar}} := {{$mod.ImplAlias}}.{{.Constructor}}()
{{end}}	{{$mod.Alias}}.RegisterModule(r, grpcServer, {{if $mod.NeedsAuthz}}authInst, permInst, {{end}}{{$mod.Alias}}.ModuleImpls{
{{range $mod.Services}}		{{.Name}}: {{.ImplVar}},
{{end}}	})

	return nil
}
{{end}}`

// renderModulesContent renders modulesTemplate against data, prepending the
// stamp header before gofmt so the output is gofmt-clean on disk.
func renderModulesContent(tag string, data serverData) ([]byte, error) {
	content, err := renderGoFile(tag, "app "+modulesFileName, modulesTemplate, data)
	if err != nil {
		return nil, err
	}

	return content, nil
}
