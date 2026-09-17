package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

const newUsage = `zever new <name> [--module PATH] [--dir PATH] [--framework-path PATH] [--framework-version V] [--force]

Scaffolds a brand new, buildable zever application from scratch: a fresh
go.mod, a starter schema/app.zen (one User entity), cmd/server + cmd/worker +
db/seed entrypoints, and internal/app/app.go, all at the exact default
locations project.go's ProjectConfig assumes -- so every other zever
subcommand (serve, dev, queue:work, db migrate/seed, tinker) works against
the new project with no zever.yaml/.json needed.

There is no tagged zever release yet, so by default the new project depends
on zever via a local ` + "`replace`" + ` directive against a checkout on disk:
--framework-path names it explicitly, or it is auto-detected by walking up
from the working directory looking for a go.mod declaring
"module github.com/zenta-dev/zever" (the same thing you get for free when
running this from inside a clone of the framework itself). Pass
--framework-version instead to depend on a real published version with no
replace directive, once one exists.

What is NOT generated: real HTTP route handlers and real job logic. Every
entrypoint this command writes has the same TODO-stub ceiling as
` + "`zever generate server`" + `/` + "`worker`" + `/` + "`seed`" + `: this gets you to a buildable,
runnable skeleton in one command, not to a finished app.

Flags:`

// Framework wiring shared by `zever new` and `zever extract`: there is no
// tagged release yet, so scaffolds resolve the framework to a local checkout
// (replace directive) unless a real version is requested.
const (
	// frameworkModulePath is zever's own module path. Scaffolds depend on
	// it: generated code imports github.com/zenta-dev/zever/... packages,
	// resolved by the scaffolded module's replace directive.
	frameworkModulePath = "github.com/zenta-dev/zever"

	// grpcGoToolModulePath is the Go tool dependency a scaffolded go.mod
	// needs so that "go tool protoc-gen-go-grpc" works out of the box.
	grpcGoToolModulePath = "google.golang.org/grpc/cmd/protoc-gen-go-grpc"

	// defaultGoVersion is used when no go directive can be read from the
	// framework checkout or an existing project.
	defaultGoVersion = "1.24"

	// pseudoVersionZero is the placeholder version a require line carries
	// when the real resolution comes from a local replace directive.
	pseudoVersionZero = "v0.0.0-00010101000000-000000000000"
)

// NewConfig is the pure input to writeNewProject: everything the scaffold
// needs, resolved before anything is written, so a failure to work out the
// target leaves no half-written directory. The TUI screens (Wave 3) build
// this struct directly and call writeNewProject; runNew builds it from CLI
// flags. It performs no I/O itself.
//
// Fields fall into two groups. Inputs (set by the caller): Name, OutDir,
// ModulePath, Force, DBAdapter, CacheAdapter, QueueAdapter, Batteries,
// Backends. Resolved (filled by runNew's framework/existing-project
// detection; screens leave them for the same resolution or set them
// explicitly): GoVersion, FrameworkDir, FrameworkVersion, ExistingProject.
// Batteries is never left empty by runNew: empty means "core only".
type NewConfig struct {
	Name             string
	OutDir           string
	ModulePath       string
	GoVersion        string
	FrameworkDir     string // local checkout, absolute, or "" when using a real version
	FrameworkVersion string
	Force            bool
	ExistingProject  bool // cwd already has a go.mod; skip go.mod generation

	// DBAdapter, CacheAdapter and QueueAdapter override the corresponding
	// entries in the service map written to zever.yaml. Empty means "use
	// config.Default()'s pick" (sqlite / memory / memory). Only ever
	// populated by the TUI wizard — the flag-driven path has no equivalent
	// flags and always leaves these empty.
	DBAdapter    string
	CacheAdapter string
	QueueAdapter string

	// Batteries is the full set of service names to write into zever.yaml
	// and blank-import into internal/app/app.go: always coreBatteries
	// (generate_server.go) plus whatever extra services were opted into.
	// Set by runNew before writeNewProject runs -- never left as the zero
	// value, so a nil/empty Batteries elsewhere in the pipeline is a bug,
	// not "use the default".
	Batteries []string

	// Backends is the set of backend names the scaffolded project's
	// README/next-steps quickstart recommends for its first `zever compile`
	// invocation. Empty means "use the historical default sample"
	// (zenorm, proto, atlas) — `zever compile` itself already defaults
	// to every registered backend regardless of this field; this only
	// steers what the freshly scaffolded project's docs suggest running
	// first, since running every backend against an empty starter schema is
	// needless output for a brand new project.
	Backends []string
}

// defaultNewBackends is the historical sample --backend list suggested to a
// freshly scaffolded project when no narrower set was picked (i.e. the
// flag-driven `zever new` path, which has no backend-picking flags of its
// own).
var defaultNewBackends = []string{"zenorm", "proto", "atlas"}

// quickstartBackends returns cfg.Backends joined with "," if set, or the
// default sample otherwise -- the single place the README/next-steps
// quickstart command's --backend value is computed.
func quickstartBackends(cfg NewConfig) string {
	if len(cfg.Backends) == 0 {
		return strings.Join(defaultNewBackends, ",")
	}

	return strings.Join(cfg.Backends, ",")
}

// runNew scaffolds a new project from flags only. There is no interactive
// path here: no wizard, no huh prompts, no TTY checks. Guided input lives in
// the TUI screens, which build a NewConfig and call writeNewProject
// directly; this stays the non-interactive, flag-driven entry point.
func runNew(args []string) error {
	const tag = "zever new"

	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	modulePath := fs.String("module", "", "Go module path for the new project (default: the app name itself, see -h)")
	dirFlag := fs.String("dir", "", "output directory (default ./<name>)")
	frameworkPath := fs.String("framework-path", "",
		"path to a local zever checkout to replace against (default: auto-detected)")
	frameworkVersion := fs.String("framework-version", "",
		"depend on a published zever version instead of a local replace directive")
	force := fs.Bool("force", false, "scaffold into a non-empty directory anyway")

	fs.Usage = func() {
		_, _ = fmt.Fprintln(fs.Output(), newUsage)
		fs.PrintDefaults()
	}

	positional, err := flexibleParse(fs, args)
	if err != nil {
		return err
	}

	if len(positional) != 1 {
		fs.Usage()

		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("try 'zever new myapp'"))
		}

		return fmt.Errorf("%s: expected exactly one app name, got %d", tag, len(positional))
	}

	name := positional[0]
	if !isValidAppName(name) {
		return fmt.Errorf("%s: %q is not a valid app name (letters, digits, - and _ only)", tag, name)
	}

	if *frameworkPath != "" && *frameworkVersion != "" {
		return fmt.Errorf("%s: --framework-path and --framework-version are mutually exclusive", tag)
	}

	outDir := *dirFlag
	if outDir == "" {
		outDir = "./" + name
	}

	// The flag-driven path has no battery picker: the scaffold is the core
	// floor set, exactly what every generated entrypoint needs.
	plan := NewConfig{
		Name:      name,
		OutDir:    outDir,
		Force:     *force,
		Batteries: batteriesFor(nil, ""),
	}

	plan.ModulePath = *modulePath
	explicitModule := *modulePath != ""

	if plan.ModulePath == "" {
		// Unlike `zever extract`, which derives a module path from an
		// existing project's go.mod, `new` has nothing to derive from and no
		// tagged release exists yet to imply a plausible publishing domain.
		// The app name alone is a legal go.mod module path and builds fine
		// against a local replace directive; anyone intending to publish the
		// module overrides this with --module.
		plan.ModulePath = name
	}

	// Detect an existing Go/Cargo project in cwd before any framework
	// resolution. This makes `zever new` usable inside an already-initialised
	// Go module (skip go.mod, derive module path) and gives a clear error
	// for Rust manifests.
	cwdHasGoMod := false
	if _, goModErr := os.Stat("go.mod"); goModErr == nil {
		cwdHasGoMod = true
	} else if !os.IsNotExist(goModErr) {
		return fmt.Errorf("%s: stat go.mod: %w", tag, goModErr)
	}

	cwdHasCargo := false
	if _, cargoErr := os.Stat("Cargo.toml"); cargoErr == nil {
		cwdHasCargo = true
	} else if !os.IsNotExist(cargoErr) {
		return fmt.Errorf("%s: stat Cargo.toml: %w", tag, cargoErr)
	}

	switch {
	case cwdHasGoMod:
		plan.ExistingProject = true

		if !explicitModule {
			if mp := modulePathOf("go.mod"); mp != "" {
				plan.ModulePath = mp
			}
		}
		// Preserve the toolchain version from the existing go.mod when
		// possible; otherwise fall back to the default. Framework checkout
		// detection is unnecessary when we are not writing a go.mod.
		if v := goVersionOf("go.mod"); v != "" {
			plan.GoVersion = v
		} else {
			plan.GoVersion = defaultGoVersion
		}
	case cwdHasCargo:
		return fmt.Errorf("%s: Rust scaffolding not supported yet; zever new targets Go projects", tag)
	default:
		if fwErr := resolveFramework(tag, &plan, *frameworkPath, *frameworkVersion); fwErr != nil {
			return fwErr
		}
	}

	if targetErr := ensureTargetDir(tag, outDir, *force); targetErr != nil {
		return targetErr
	}

	written, err := writeNewProject(tag, plan)
	if err != nil {
		return err
	}

	printNewSummary(plan, written)

	return nil
}

// isValidAppName reports whether s is safe to use as a default directory
// name and go.mod module path component: non-empty, no path separators, no
// characters that would make either illegal or surprising. In particular it
// rejects "..", "/" and "\" so a crafted name can never escape the target
// directory (path-traversal confinement for the scaffold root).
func isValidAppName(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}

	return true
}

// resolveFramework decides how the new project depends on zever: a real
// version (--framework-version, no replace directive) or a local checkout
// (--framework-path, or auto-detected by walking up from the working
// directory) with a replace directive, mirroring the decision
// `zever extract`'s readGoMod/resolveFrameworkDir already make for an
// existing project, adapted to the case where no go.mod exists yet.
func resolveFramework(tag string, plan *NewConfig, frameworkPath, frameworkVersion string) error {
	if frameworkVersion != "" {
		plan.FrameworkVersion = frameworkVersion
		plan.GoVersion = defaultGoVersion

		return nil
	}

	fwDir := frameworkPath
	explicit := fwDir != ""

	if fwDir == "" {
		detected, err := detectFrameworkCheckout()
		if err != nil {
			return fmt.Errorf("%s: %w", tag, err)
		}

		fwDir = detected
	}

	// Nothing explicit and nothing found by walking up: default to the
	// working directory itself rather than erroring, so `zever new <name>`
	// works with zero flags as the easiest onboarding path. This is a best
	// effort guess, not a validated checkout -- if it's wrong the generated
	// go.mod's replace directive is a one-line hand edit away from fixed.
	usingCwdDefault := false

	if fwDir == "" {
		fwDir = "."
		usingCwdDefault = true
	}

	abs, err := filepath.Abs(fwDir)
	if err != nil {
		return fmt.Errorf("%s: %w", tag, err)
	}

	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		if !explicit && usingCwdDefault {
			// No go.mod at the default guess either -- still don't block
			// scaffolding. Point the replace directive at cwd anyway; the
			// user edits it by hand once they know where zever actually
			// lives, per --framework-path's own doc comment above.
			plan.FrameworkDir = abs
			plan.FrameworkVersion = pseudoVersionZero
			plan.GoVersion = defaultGoVersion

			return nil
		}

		return fmt.Errorf("%s: %q does not look like a zever checkout (no go.mod): %w", tag, fwDir, err)
	}

	plan.FrameworkDir = abs
	plan.FrameworkVersion = pseudoVersionZero
	plan.GoVersion = detectGoVersion(abs)

	return nil
}

// detectFrameworkCheckout walks up from the working directory looking for a
// go.mod declaring "module github.com/zenta-dev/zever" -- the same clone
// this command itself is most likely being run from during framework
// development, since there is no tagged release to depend on any other way.
// Returns "" (not an error) when no such go.mod is found before the
// filesystem root.
func detectFrameworkCheckout() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}

	for {
		if modulePathOf(filepath.Join(dir, "go.mod")) == frameworkModulePath {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}

		dir = parent
	}
}

// modulePathOf reads the module path out of the go.mod at path, returning ""
// if the file does not exist or declares none.
func modulePathOf(path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // walking parent directories looking for a go.mod is the point
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		mp, ok := strings.CutPrefix(line, "module")
		if !ok {
			continue
		}

		mp = strings.TrimSpace(mp)
		if mp == "" || mp == line {
			continue
		}

		return strings.Trim(mp, `"`)
	}

	return ""
}

// detectGoVersion reads the `go` directive out of frameworkDir/go.mod, so a
// locally-replaced project matches the toolchain version the framework
// itself builds with. Falls back to defaultGoVersion (also extract.go's
// fallback) if that cannot be read.
func detectGoVersion(frameworkDir string) string {
	data, err := os.ReadFile(filepath.Join(frameworkDir, "go.mod")) //nolint:gosec // developer-supplied checkout path
	if err != nil {
		return defaultGoVersion
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		if v, ok := strings.CutPrefix(line, "go "); ok {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}

	return defaultGoVersion
}

// goVersionOf reads the `go` directive out of the go.mod at path, returning ""
// if the file cannot be read or declares none. Used to preserve the existing
// project's toolchain version when scaffolding into a cwd that already has a
// go.mod (so we don't invent a version).
func goVersionOf(path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // path is cwd go.mod, not user input
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		if v, ok := strings.CutPrefix(line, "go "); ok {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}

	return ""
}

// batteryYAML is the on-disk shape for a service entry; Options is
// omitted when empty to match the hand-written examples.
type batteryYAML struct {
	Adapter string         `yaml:"adapter"`
	Options map[string]any `yaml:"options,omitempty"`
}

// allServiceAdapters is every service config.Default() knows, with its
// default adapter. It is the single source for nonCoreBatteryNames and
// defaultServiceAdapter, so the scaffold's service universe cannot drift
// from the runtime's. TestAllServiceAdaptersMatchDefault guards the drift.
//
// Adaptation note: zever's config is typed (Service[T] fields, no Get
// map), so the defaults are mirrored here field by field instead of read
// through a map lookup.
var allServiceAdapters = map[string]string{
	"ai":            "anthropic",
	"analytics":     "log",
	"auth":          "jwt",
	"billing":       "stub",
	"cache":         "memory",
	"crypto":        "local",
	"db":            "sqlite",
	"document":      "local",
	"eventbus":      "memory",
	"flag":          "static",
	"geo":           "static",
	"i18n":          "embed",
	"idempotency":   "memory",
	"log":           "slog",
	"mailer":        "log",
	"media":         "local",
	"notification":  "log",
	"observability": "stdout",
	"password":      "argon2id",
	"payment":       "stub",
	"permission":    "noop",
	"queue":         "memory",
	"ratelimit":     "memory",
	"router":        "stdhttp",
	"scheduler":     "embedded",
	"search":        "sqlite",
	"session":       "memory",
	"storage":       "local",
	"tenant":        "single",
	"vectorstore":   "sqlite",
	"webhook":       "http",
	"workflow":      "memory",
}

// defaultServiceAdapter returns config.Default()'s adapter pick for the
// named service, or "" when the name is not a known service.
func defaultServiceAdapter(name string) string {
	if a, ok := allServiceAdapters[name]; ok {
		return a
	}

	// Belt and braces: the map above is the source of truth, but a service
	// added to config.Default without updating it must not silently scaffold
	// an empty adapter. Fall back to the live default.
	if sc, ok := config.Default().RedactedServices()[name]; ok {
		return sc.Adapter
	}

	return ""
}

// batteriesFor merges coreBatteries with optional (developer-picked)
// batteries, adding "cache" whenever cacheAdapter is a real override --
// explicit intent to use caching even when "cache" wasn't itself picked.
func batteriesFor(optional []string, cacheAdapter string) []string {
	batteries := mergeBatteries(coreBatteries, optional)

	if cacheAdapter != "" {
		batteries = mergeBatteries(batteries, []string{"cache"})
	}

	return batteries
}

// mergeBatteries returns the sorted, de-duplicated union of core and extra:
// coreBatteries (generate_server.go) plus whatever optional batteries a
// developer picked.
func mergeBatteries(core, extra []string) []string {
	set := make(map[string]bool, len(core)+len(extra))

	for _, b := range core {
		set[b] = true
	}

	for _, b := range extra {
		set[b] = true
	}

	out := make([]string, 0, len(set))
	for b := range set {
		out = append(out, b)
	}

	sort.Strings(out)

	return out
}

// nonCoreBatteryNames returns every service config.Default() knows about
// except coreBatteries, sorted -- the list offered to a developer composing
// their scaffold beyond the floor set every generated entrypoint needs.
func nonCoreBatteryNames() []string {
	core := make(map[string]bool, len(coreBatteries))
	for _, b := range coreBatteries {
		core[b] = true
	}

	out := make([]string, 0, len(allServiceAdapters))

	for name := range allServiceAdapters {
		if !core[name] {
			out = append(out, name)
		}
	}

	sort.Strings(out)

	return out
}

// batterySelections resolves cfg.Batteries into batterySelection pairs,
// applying cfg.DBAdapter/CacheAdapter/QueueAdapter overrides -- the single
// source both renderZeverYaml and writeNewProject's app.go render use, so
// the two files can never pick different adapters for the same service.
func (c NewConfig) batterySelections() []batterySelection {
	overrideFor := map[string]string{"db": c.DBAdapter, "cache": c.CacheAdapter, "queue": c.QueueAdapter}

	sel := make([]batterySelection, 0, len(c.Batteries))

	for _, name := range c.Batteries {
		adapter := defaultServiceAdapter(name)
		if o := overrideFor[name]; o != "" {
			adapter = o
		}

		sel = append(sel, batterySelection{Battery: name, Adapter: adapter})
	}

	return sel
}

// renderZeverYaml builds the always-emitted zever.yaml: exactly
// cfg.Batteries' adapter picks (never the full config.Default() service
// map -- see NewConfig.Batteries' own doc comment for why the set is
// minimal by default and developer-composed beyond that).
//
// Adaptation note: the "project" table zengo wrote alongside the batteries
// is deliberately omitted. zever's config.Load is strict and rejects a
// "project" key, so a generated file containing one would fail to load in
// the scaffolded project itself. loadProjectConfig assumes the defaults
// below for a missing table, which is exactly the layout writeNewProject
// scaffolds, so nothing is lost.
func renderZeverYaml(cfg NewConfig) ([]byte, error) {
	batteries := make(map[string]batteryYAML, len(cfg.Batteries))
	for _, sel := range cfg.batterySelections() {
		batteries[sel.Battery] = batteryYAML{Adapter: sel.Adapter}
	}

	data, err := yaml.Marshal(batteries)
	if err != nil {
		return nil, fmt.Errorf("[zever new] marshal zever.yaml: %w", err)
	}

	return data, nil
}

// ensureTargetDir refuses to scaffold into a non-empty existing directory
// unless force is set, mirroring the collision-protection convention
// writeScaffold already applies per-file. A directory that does not exist
// yet is created.
func ensureTargetDir(tag, dir string, force bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			if mkdirErr := os.MkdirAll(dir, 0o750); mkdirErr != nil { //nolint:gosec // dir is the CLI's own --dir/positional arg, developer-supplied
				return fmt.Errorf("%s: mkdir %q: %w", tag, dir, mkdirErr)
			}

			return nil
		}

		return fmt.Errorf("%s: read %q: %w", tag, dir, err)
	}

	if len(entries) > 0 && !force {
		return fmt.Errorf("%s: %q already exists and is not empty (pass --force to scaffold into it anyway)", tag, dir)
	}

	return nil
}

// writeNewSeedFiles renders and writes db/seed/main.go plus its
// skip-if-exists seed.go stub, factored out of writeNewProject purely to
// keep that function's cyclomatic complexity down -- see writeExtractedGoFiles
// and runGenerateSeed for the same two-file split.
func writeNewSeedFiles(tag string, cfg NewConfig, write func(rel string, content []byte) error) error {
	seedContent, err := renderGoFile(tag, "seed main.go", seedTemplate, struct{ ModulePath string }{cfg.ModulePath})
	if err != nil {
		return err
	}

	if writeErr := write(filepath.Join(defaultSeedEntry, "main.go"), seedContent); writeErr != nil {
		return writeErr
	}

	seedStubContent, err := renderGoFile(tag, "seed stub", seedStubTemplate, struct{ GeneratedDir string }{defaultGeneratedDir})
	if err != nil {
		return err
	}

	return write(filepath.Join(seedStubRoot, "seed.go"), seedStubContent)
}

// writeNewProject materialises a planned new project on disk, at exactly the
// paths ProjectConfig's defaults assume. It is THE single file-writing path
// for scaffolding: runNew (flags) and the TUI screens (Wave 3) both converge
// here, so every scaffold writes the same tree.
func writeNewProject(tag string, cfg NewConfig) ([]string, error) {
	written := make([]string, 0, 10)

	write := func(rel string, content []byte) error {
		path := filepath.Join(cfg.OutDir, rel)

		if err := writeScaffold(tag, path, content, cfg.Force); err != nil {
			return err
		}

		written = append(written, path)

		return nil
	}

	if !cfg.ExistingProject {
		modFile, err := renderNewGoMod(tag, cfg)
		if err != nil {
			return nil, err
		}

		if writeErr := write("go.mod", modFile); writeErr != nil {
			return nil, writeErr
		}
	}

	if err := write(filepath.Join(defaultSchemaDir, "app.zen"), []byte(renderNewSchema(cfg.Name))); err != nil {
		return nil, err
	}

	appContent, err := renderAppContent(tag, cfg.batterySelections())
	if err != nil {
		return nil, err
	}

	if writeErr := write(filepath.Join("internal", "app", "app.go"), appContent); writeErr != nil {
		return nil, writeErr
	}

	serverContent, err := renderGoFile(tag, "server main.go", serverTemplate, serverData{ModulePath: cfg.ModulePath})
	if err != nil {
		return nil, err
	}

	if writeErr := write(filepath.Join(defaultServerEntry, "main.go"), serverContent); writeErr != nil {
		return nil, writeErr
	}

	workerContent, err := renderGoFile(tag, "worker main.go", workerTemplate, buildWorkerData(cfg.ModulePath, &ir.Schema{}))
	if err != nil {
		return nil, err
	}

	if writeErr := write(filepath.Join(defaultWorkerEntry, "main.go"), workerContent); writeErr != nil {
		return nil, writeErr
	}

	if seedErr := writeNewSeedFiles(tag, cfg, write); seedErr != nil {
		return nil, seedErr
	}

	if writeErr := write(".gitignore", []byte(newGitignore)); writeErr != nil {
		return nil, writeErr
	}

	if writeErr := write("README.md", []byte(renderNewReadme(cfg.Name, quickstartBackends(cfg)))); writeErr != nil {
		return nil, writeErr
	}

	// app.go's DefaultDBPath is data/app.db; keeping the directory in the
	// tree means the scaffolded binaries run without a mkdir first (as
	// zever extract already does for extracted services).
	if writeErr := write(filepath.Join("data", ".gitkeep"), nil); writeErr != nil {
		return nil, writeErr
	}

	yamlData, err := renderZeverYaml(cfg)
	if err != nil {
		return nil, err
	}

	if writeErr := write("zever.yaml", yamlData); writeErr != nil {
		return nil, writeErr
	}

	return written, nil
}

// renderNewGoMod writes the new project's go.mod: a local replace directive
// against a zever checkout by default (there is no tagged release yet,
// exactly as zever extract's own output notes), or a plain require of a
// real version when --framework-version was given.
func renderNewGoMod(tag string, cfg NewConfig) ([]byte, error) {
	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "module %s\n\ngo %s\n\n", cfg.ModulePath, cfg.GoVersion)

	if cfg.FrameworkDir != "" {
		rel, err := relativeTo(cfg.OutDir, cfg.FrameworkDir)
		if err != nil {
			return nil, fmt.Errorf("%s: locate the framework from %q: %w", tag, cfg.OutDir, err)
		}

		_, _ = fmt.Fprintf(&b, "// There is no tagged zever release yet, so `zever new` defaults to a local\n"+
			"// replace directive against a zever checkout; pass --framework-version to\n"+
			"// depend on a real published version instead.\nreplace %s => %s\n\n", frameworkModulePath, rel)
	}

	_, _ = fmt.Fprintf(&b, "require %s %s\n", frameworkModulePath, cfg.FrameworkVersion)

	_, _ = fmt.Fprintf(&b, "\n// protogogen's grpc codegen shells out to this pinned tool dependency\n"+
		"// (\"go tool protoc-gen-go-grpc\") instead of requiring a protoc-gen-go-grpc\n"+
		"// binary on PATH; `go mod tidy` resolves its version and go.sum entries\n"+
		"// automatically.\ntool %s\n",
		grpcGoToolModulePath)

	return []byte(b.String()), nil
}

// renderNewSchema is the starter schema/app.zen: one minimal entity so the
// generated project has something real for `zever compile`/`db migrate` to
// act on, matching the convention project.go's SchemaDir default expects.
func renderNewSchema(name string) string {
	return fmt.Sprintf(`// app.zen -- the starting schema for %s.
//
// Add entities, services, jobs and schedules here, then regenerate the code
// that is derived from this file:
//
//	zever compile schema/*.zen --backend=zenorm,proto,atlas
//	zever db migrate schema/*.zen --adapter=sqlite --dsn=data/app.db

entity User {
	id: uuid @primary
	email: string @unique
	created_at: timestamp @default(now())
}
`, name)
}

// newGitignore mirrors the one real rule a fresh project needs (the local
// sqlite file db migrate creates is never committed) plus the generated-code
// directory a fresh project has no reason to check in.
const newGitignore = `# The local sqlite database is created by ` + "`zever db migrate`" + `; never commit it.
/data/*.db

# Regenerated from schema/*.zen by ` + "`zever compile`" + ` -- see README.
/generated/
`

// renderNewReadme is a genuine quickstart, not filler: every command in it
// is one a freshly scaffolded project can actually run, in order.
func renderNewReadme(name, backends string) string {
	const backtick = "`"

	tmpl := `# %[1]s

A zever application, scaffolded by ` + backtick + `zever new` + backtick + `. One ` + backtick + `.zen` + backtick + ` schema
(` + backtick + `schema/app.zen` + backtick + `) is the source of truth; the ` + backtick + `zever` + backtick + ` CLI turns it into a
query builder, a Protobuf service definition and the SQL schema, and
everything else is resolved from a ` + backtick + `container.Container` + backtick + ` built in
` + backtick + `internal/app/app.go` + backtick + `.

## Quickstart

` + "```bash" + `
go mod tidy

# turn schema/app.zen into a query builder (internal/orm), a Protobuf
# service definition (generated/proto) and SQL DDL
zever compile --backend=%[2]s schema/app.zen

# create the sqlite database from the schema (safe to re-run)
zever db migrate --adapter=sqlite --dsn=data/app.db schema/app.zen

# run the HTTP API (-addr to change the listen address)
zever serve

# in another shell: the job worker + embedded scheduler
zever queue:work

# watch schema and server source; recompile and restart on change
zever dev
` + "```" + `

## What's here, and what's not

- ` + backtick + `schema/app.zen` + backtick + ` -- one starter ` + backtick + `User` + backtick + ` entity. Extend it with
  ` + backtick + `zever generate entity/job/schedule` + backtick + `.
- ` + backtick + `cmd/server/main.go` + backtick + `, ` + backtick + `cmd/worker/main.go` + backtick + `,
  ` + backtick + `db/seed/main.go` + backtick + ` -- thin entrypoints, the same shape
  ` + backtick + `zever generate server/worker/seed` + backtick + ` produce.
- ` + backtick + `internal/app/app.go` + backtick + ` -- the one place this project selects its
  adapters and builds its container.

Route handlers and job bodies are **not** generated -- add RPCs and jobs to
the schema, then wire the actual logic by hand, same TODO-stub ceiling as
every ` + backtick + `zever generate` + backtick + ` scaffold. ` + backtick + `zever new` + backtick + ` gets you to a buildable,
runnable skeleton in one command, not to a finished app.
`

	return fmt.Sprintf(tmpl, name, backends)
}

func printNewSummary(cfg NewConfig, written []string) {
	// Header
	_, _ = fmt.Fprintln(os.Stdout, success("✔ scaffolded ")+bold(fmt.Sprintf("%q", cfg.Name))+dim(" into ")+cyan(cfg.OutDir))
	_, _ = fmt.Fprintln(os.Stdout, "  "+dim("module path: ")+cyan(cfg.ModulePath))

	if cfg.FrameworkDir != "" {
		_, _ = fmt.Fprintln(os.Stdout, "  "+dim("zever: ")+dim("local replace → ")+cyan(cfg.FrameworkDir))
	} else {
		_, _ = fmt.Fprintln(os.Stdout, "  "+dim("zever: ")+cyan(cfg.FrameworkVersion))
	}

	for _, path := range written {
		_, _ = fmt.Fprintln(os.Stdout, "  "+dim("wrote ")+cyan(path))
	}

	next := joinLines(
		bold("Next steps:"),
		"  "+dim("1.")+"  "+cmd("cd "+cfg.OutDir),
		"  "+dim("2.")+"  "+cmd("go mod tidy"),
		"  "+dim("3.")+"  "+cmd("zever compile --backend="+quickstartBackends(cfg)+" schema/app.zen"),
		"  "+dim("4.")+"  "+cmd("zever db migrate --adapter=sqlite --dsn=data/app.db schema/app.zen"),
		"  "+dim("5.")+"  "+cmd("zever serve"),
	)

	_, _ = fmt.Fprintln(os.Stdout, "")

	if colorEnabled {
		_, _ = fmt.Fprintln(os.Stdout, box(next))
	} else {
		_, _ = fmt.Fprintln(os.Stdout, next)
	}

	if shouldShowHint() {
		_, _ = fmt.Fprintln(os.Stderr, formatHint(hintFor("new")))
	}
}
