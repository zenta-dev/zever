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
	"github.com/zenta-dev/zever/dsl/ir"
)

const newUsage = `zever new <name> [--module PATH] [--dir PATH] [--framework-version V] [--force]
                     [--interactive] [--batteries a,b] [--adapters b1=a1,b2=a2] [-y|--yes] [--list-batteries]

Scaffolds a brand new, buildable zever application from scratch: a fresh
go.mod, a starter schema/app.zen (one User entity), cmd/server + cmd/worker +
db/seed entrypoints, and internal/app/app.go, all at the exact default
locations project.go's ProjectConfig assumes -- so every other zever
subcommand (serve, dev, queue:work, db migrate/seed, tinker) works against
the new project with no zever.yaml/.json needed.

By default the new project depends on the published zever module
(github.com/zenta-dev/zever %s) with no replace directive. Pass
--framework-version to pin a different published version. When run from
inside a clone of the zever framework itself (a go.mod declaring "module
github.com/zenta-dev/zever" somewhere above the working directory), the
scaffold automatically replaces against that local checkout instead --
there is no flag for this since it only ever applies to framework
development, never to an ordinary project.

What is NOT generated: real HTTP route handlers and real job logic. Every
entrypoint this command writes has the same TODO-stub ceiling as
` + "`zever generate server`" + `/` + "`worker`" + `/` + "`seed`" + `: this gets you to a buildable,
runnable skeleton in one command, not to a finished app.

Battery picker (OPT-IN only): without --interactive this command never
prompts and scaffolds the floor set (log+router) plus whatever --batteries
and --adapters select. Pass --interactive for the huh wizard (TTY
required): a MultiSelect over every known battery (floor pre-selected),
then one adapter Select per picked non-floor battery. Every prompt answer
has a flag: --batteries skips the battery prompt, --adapters b=a skips
that battery's adapter prompt, and -y/--yes skips the whole wizard.
--list-batteries prints the battery/default-adapter table and exits.

Flags:`

// Framework wiring shared by `zever new` and `zever extract`.
const (
	// frameworkModulePath is zever's own module path. Scaffolds depend on
	// it: generated code imports github.com/zenta-dev/zever/... packages.
	frameworkModulePath = "github.com/zenta-dev/zever"

	// grpcGoToolModulePath is the Go tool dependency a scaffolded go.mod
	// needs so that "go tool protoc-gen-go-grpc" works out of the box.
	grpcGoToolModulePath = "google.golang.org/grpc/cmd/protoc-gen-go-grpc"

	// defaultGoVersion is used when no go directive can be read from the
	// framework checkout or an existing project.
	defaultGoVersion = "1.27"

	// FrameworkVersion is the published framework version a new project
	// depends on by default and `zever add` pins new requirements to. It
	// tracks the CLI release version (see cliVersion in main.go); bump with
	// every release.
	FrameworkVersion = "v" + cliVersion

	// defaultFrameworkVersion is the published zever version a new project
	// depends on by default when neither --framework-version is given nor
	// a framework checkout is detected.
	defaultFrameworkVersion = FrameworkVersion

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
	// and import into internal/app/app.go: always coreBatteries
	// (generate_server.go's floor: log, router) plus whatever extra
	// services were opted into.
	// Set by runNew before writeNewProject runs -- never left as the zero
	// value, so a nil/empty Batteries elsewhere in the pipeline is a bug,
	// not "use the default". renderAppContent emits exactly this set
	// (floor + picks, nothing more), and the generated go.mod requires the
	// framework modules for exactly this set (container, config, one
	// core/<battery> and one adapters/<battery>/<adapter> module per
	// selection); `go mod tidy` resolves everything transitive.
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

	// Adapters overrides the adapter pick per battery name (e.g.
	// {"db":"postgres"}). Populated by the --adapters flag and the
	// --interactive picker; the flag-driven floor path leaves it empty.
	// batterySelections consults it for every battery, generalising the
	// three legacy DBAdapter/CacheAdapter/QueueAdapter overrides (which
	// still win when both are set).
	Adapters map[string]string
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

// newNewFlagSet builds the stdlib flag set for `zever new`: the single
// source for runNew parsing and for help/usage printers, so help can never
// drift from the real flags.
func newNewFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.String("module", "", "Go module path for the new project (default: the app name itself, see -h)")
	fs.String("dir", "", "output directory (default ./<name>)")
	fs.String("framework-version", "",
		"depend on a published zever version instead of a local replace directive")
	fs.Bool("force", false, "scaffold into a non-empty directory anyway")
	fs.Bool("interactive", false, "run the opt-in battery picker wizard (TTY required)")
	fs.Bool("i", false, "alias for --interactive")
	fs.String("batteries", "", "comma-separated extra batteries to scaffold (e.g. --batteries db,cache)")
	fs.String("adapters", "", "comma-separated battery=adapter pairs (e.g. --adapters db=postgres,router=fiber)")
	fs.Bool("yes", false, "skip all prompts, scaffold floor+flags only")
	fs.Bool("y", false, "alias for --yes")
	fs.Bool("list-batteries", false, "print the battery/default-adapter table to stdout and exit")
	fs.Bool("dry-run", false, "print the scaffold plan without writing any files")

	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), newUsage+"\n", defaultFrameworkVersion)
		fs.PrintDefaults()
	}

	return fs
}

// printNewUsage writes the full `zever new` help (header + real flag
// defaults) to fs.Output, for the legacy `zever help new` path.
func printNewUsage(fs *flag.FlagSet) {
	_, _ = fmt.Fprintf(fs.Output(), newUsage+"\n", defaultFrameworkVersion)
	fs.PrintDefaults()
}

// runNew scaffolds a new project from flags, with one opt-in interactive
// path: the battery picker runs ONLY when --interactive (or -i, the global
// interactive mode, or ZEVER_INTERACTIVE) is set and -y/--yes is not. Every
// other invocation is flags-only and never prompts, so bare off-TTY runs
// stay on the usage-or-scaffold path and can never hang in huh.
func runNew(args []string) error {
	const tag = "zever new"

	fs := newNewFlagSet()

	positional, err := flexibleParse(fs, args)
	if err != nil {
		return err
	}

	if flagBool(fs, "list-batteries") {
		printBatteryTable()

		return nil
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

	flagString := func(name string) string {
		if f := fs.Lookup(name); f != nil {
			return f.Value.String()
		}
		return ""
	}

	outDir := flagString("dir")
	if outDir == "" {
		outDir = "./" + name
	}

	// Flag-parity inputs: --batteries selects extra batteries, --adapters
	// overrides per-battery adapters (and implies its batteries, the same
	// way a cache override already implies "cache" in batteriesFor).
	optional, err := parseBatteriesFlag(flagString("batteries"))
	if err != nil {
		return err
	}

	overrides, err := parseAdaptersFlag(flagString("adapters"))
	if err != nil {
		return err
	}

	for b := range overrides {
		if !batteryPicked(optional, b) {
			optional = append(optional, b)
		}
	}

	// The picker is strictly opt-in: only --interactive (or the global
	// interactive mode / env) enters it, and -y/--yes always skips it.
	// Flag-supplied answers survive into the wizard: --batteries skips the
	// battery prompt, --adapters skips that battery's adapter prompt.
	if wantNewPicker(fs) {
		picked, pickedAdapters, pickerErr := runBatteryPicker(optional, overrides)
		if pickerErr != nil {
			return pickerErr
		}

		optional = picked
		overrides = pickedAdapters
	}

	// Without the picker the scaffold is the core floor set plus flags,
	// exactly what every generated entrypoint needs.
	plan := NewConfig{
		Name:      name,
		OutDir:    outDir,
		Force:     flagBool(fs, "force"),
		Batteries: batteriesFor(optional, overrides["cache"]),
		Adapters:  overrides,
	}

	plan.ModulePath = flagString("module")
	explicitModule := flagString("module") != ""

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
		if fwErr := resolveFramework(tag, &plan, flagString("framework-version")); fwErr != nil {
			return fwErr
		}
	}

	if flagBool(fs, "dry-run") {
		printNewDryRun(plan, outDir)

		return nil
	}

	if targetErr := ensureTargetDir(tag, outDir, flagBool(fs, "force")); targetErr != nil {
		return targetErr
	}

	written, err := writeNewProject(tag, plan)
	if err != nil {
		return err
	}

	printNewSummary(plan, written)

	return nil
}

// flagBool reads a bool flag from fs by name, tolerating a missing flag
// definition (false). It replaces the runNew-local closure so the picker
// gate can share it.
func flagBool(fs *flag.FlagSet, name string) bool {
	if f := fs.Lookup(name); f != nil {
		return f.Value.String() == "true"
	}

	return false
}

// wantNewPicker reports whether runNew should enter the opt-in battery
// picker: interactive input was requested (the --interactive/-i flag, the
// global peeled mode, or ZEVER_INTERACTIVE) and -y/--yes did not veto it.
// Anything else stays on the flags/floor path, which never prompts.
func wantNewPicker(fs *flag.FlagSet) bool {
	if flagBool(fs, "yes") || flagBool(fs, "y") {
		return false
	}

	if flagBool(fs, "interactive") || flagBool(fs, "i") {
		return true
	}

	return interactiveMode || envInteractive()
}

// batteryPicked reports whether name is in the picked set.
func batteryPicked(picked []string, name string) bool {
	for _, b := range picked {
		if b == name {
			return true
		}
	}

	return false
}

// parseBatteriesFlag parses --batteries (comma-separated battery names)
// into a sorted, de-duplicated list. Empty means "no extras". Unknown
// names fail closed: a typo must not silently scaffold the floor set.
func parseBatteriesFlag(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	set := map[string]bool{}

	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}

		if _, ok := allServiceAdapters[name]; !ok {
			return nil, fmt.Errorf("zever new: unknown battery %q in --batteries (see --list-batteries)", name)
		}

		set[name] = true
	}

	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}

	sort.Strings(out)

	return out, nil
}

// parseAdaptersFlag parses --adapters (comma-separated battery=adapter
// pairs) into an override map. Unknown batteries and adapters unknown to
// that battery fail closed. Empty means "no overrides".
func parseAdaptersFlag(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}

	out := map[string]string{}

	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		battery, adapter, ok := strings.Cut(part, "=")
		battery = strings.TrimSpace(battery)
		adapter = strings.TrimSpace(adapter)

		if !ok || battery == "" || adapter == "" {
			return nil, fmt.Errorf("zever new: malformed --adapters entry %q, want battery=adapter", part)
		}

		if _, known := allServiceAdapters[battery]; !known {
			return nil, fmt.Errorf("zever new: unknown battery %q in --adapters (see --list-batteries)", battery)
		}

		choices := adaptersForBattery(battery)
		found := false

		for _, c := range choices {
			if c == adapter {
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("zever new: unknown adapter %q for battery %q (choices: %s)",
				adapter, battery, strings.Join(choices, ", "))
		}

		out[battery] = adapter
	}

	return out, nil
}

// batteryAdapters enumerates every battery's known adapters from
// nestedModuleDirs (the same source renderNewGoMod's replace set uses), so
// the picker's offer list cannot drift from the scaffold's module graph.
// The password adapter dir ("argon2") maps back to its PHC-canonical
// adapter string ("argon2id", mirroring adapterDirName) and the resilience
// "inproc" dir maps back to its "memory" adapter string; the consolidated
// DB-backed dirs ("search/db", "vectorstore/db", "workflow/db") map back
// to their canonical "db" adapter name, with the legacy alias names
// ("postgres", "pgvector", "sqlite") appended so old picks still validate
// in --adapters and `zever add`; every battery's default adapter is
// included even if its module dir were ever missing.
func batteryAdapters() map[string][]string {
	out := map[string][]string{}

	for _, dir := range nestedModuleDirs {
		rest, ok := strings.CutPrefix(dir, "adapters/")
		if !ok {
			continue
		}

		battery, adapterDir, ok := strings.Cut(rest, "/")
		if !ok || battery == "" || adapterDir == "" {
			continue
		}

		adapter := adapterDir
		if battery == "password" && adapterDir == "argon2" {
			adapter = defaultServiceAdapter(battery)
		}

		if battery == "resilience" && adapterDir == "inproc" {
			adapter = defaultServiceAdapter(battery)
		}

		// Consolidated DB-backed dirs serve the canonical "db" name; the
		// legacy per-backend names stay pickable as aliases (mirroring
		// adapterDirName, which routes all of them to the same dir).
		if adapterDir == "db" && (battery == "search" || battery == "vectorstore" || battery == "workflow") {
			adapter = "db"
		}

		if !batteryPicked(out[battery], adapter) {
			out[battery] = append(out[battery], adapter)
		}
	}

	// Legacy DB-backed alias names: no module dir of their own (they share
	// the consolidated db dir), but they must stay valid picks so old
	// --adapters values and `zever add` invocations keep working.
	for battery, aliases := range map[string][]string{
		"search":      {"postgres", "sqlite"},
		"vectorstore": {"pgvector", "sqlite"},
		"workflow":    {"postgres"},
	} {
		for _, alias := range aliases {
			if !batteryPicked(out[battery], alias) {
				out[battery] = append(out[battery], alias)
			}
		}
	}

	for battery, def := range allServiceAdapters {
		if !batteryPicked(out[battery], def) {
			out[battery] = append(out[battery], def)
		}
	}

	for battery := range out {
		def := defaultServiceAdapter(battery)
		rest := make([]string, 0, len(out[battery]))

		for _, a := range out[battery] {
			if a != def {
				rest = append(rest, a)
			}
		}

		sort.Strings(rest)
		out[battery] = append([]string{def}, rest...)
	}

	return out
}

// adaptersForBattery returns the known adapters for one battery, default
// first. Unknown batteries yield just the live default (or empty).
func adaptersForBattery(battery string) []string {
	if choices, ok := batteryAdapters()[battery]; ok {
		return choices
	}

	if def := defaultServiceAdapter(battery); def != "" {
		return []string{def}
	}

	return nil
}

// printBatteryTable writes the battery/default-adapter table to stdout:
// one sorted "name adapter" line per known service, floor batteries first.
// It backs --list-batteries, which exits 0 without scaffolding anything.
func printBatteryTable() {
	floor := map[string]bool{}
	for _, b := range coreBatteries {
		floor[b] = true
	}

	names := make([]string, 0, len(allServiceAdapters))
	for name := range allServiceAdapters {
		names = append(names, name)
	}

	sort.Slice(names, func(i, j int) bool {
		if floor[names[i]] != floor[names[j]] {
			return floor[names[i]]
		}

		return names[i] < names[j]
	})

	_, _ = fmt.Fprintln(os.Stdout, "BATTERY ADAPTER")
	for _, name := range names {
		marker := ""
		if floor[name] {
			marker = " (floor)"
		}

		_, _ = fmt.Fprintf(os.Stdout, "%s %s%s\n", name, defaultServiceAdapter(name), marker)
	}
}

// pickBatteriesFunc and pickAdapterFunc are seams so tests drive the picker
// headless: promptMultiSelectDefault/promptSelect take the identical path
// in production.
var pickBatteriesFunc = func(options, selected []string) ([]string, error) {
	return promptMultiSelectDefault("Extra batteries (floor log+router pre-selected)", options, selected)
}

var pickAdapterFunc = func(battery string, options []string) (string, error) {
	return promptSelect("Adapter for "+battery+" (default "+options[0]+")", options)
}

// runBatteryPicker runs the opt-in huh wizard. preselected/trumped carry
// the --batteries/--adapters answers: a supplied battery list skips the
// MultiSelect, a supplied adapter skips that battery's Select (TanStack
// rule: every prompt answer has a flag; flags skip their prompts).
// Single-adapter batteries never prompt. Off-TTY the prompt helpers fail
// via requireInteractive instead of hanging. It returns the full picked
// battery list (floor selections honoured as picked) and the merged
// adapter overrides.
func runBatteryPicker(preselected []string, trumped map[string]string) ([]string, map[string]string, error) {
	if trumped == nil {
		trumped = map[string]string{}
	}

	picked := preselected
	if picked == nil {
		options := append(append([]string(nil), coreBatteries...), nonCoreBatteryNames()...)

		sel, err := pickBatteriesFunc(options, coreBatteries)
		if err != nil {
			return nil, nil, err
		}

		picked = sel
	}

	merged := map[string]string{}
	for b, a := range trumped {
		merged[b] = a
	}

	core := map[string]bool{}
	for _, b := range coreBatteries {
		core[b] = true
	}

	for _, battery := range picked {
		if core[battery] {
			continue
		}

		if _, ok := merged[battery]; ok {
			continue
		}

		choices := adaptersForBattery(battery)
		if len(choices) <= 1 {
			continue
		}

		adapter, err := pickAdapterFunc(battery, choices)
		if err != nil {
			return nil, nil, err
		}

		merged[battery] = adapter
	}

	return picked, merged, nil
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

// zeverFrameworkPathEnv is an unexported test/dogfooding seam: when set, it
// overrides framework-checkout auto-detection with an explicit path. It is
// intentionally not a CLI flag -- ordinary users of the published module
// never need it, and this repo's own test suite is the only expected
// consumer (tests run with a cwd outside the repo tree via t.TempDir(),
// so walk-up auto-detection alone can't find the checkout to replace
// against, which is what keeps scaffolded-project builds hermetic in CI).
const zeverFrameworkPathEnv = "ZEVER_FRAMEWORK_PATH"

// resolveFramework decides how the new project depends on zever: a real
// version (--framework-version, or defaultFrameworkVersion when
// --framework-version is not given and no checkout is detected) with no
// replace directive, or a local checkout (auto-detected by walking up from
// the working directory, or overridden via ZEVER_FRAMEWORK_PATH for this
// repo's own tests) with a replace directive.
func resolveFramework(tag string, plan *NewConfig, frameworkVersion string) error {
	if frameworkVersion != "" {
		plan.FrameworkVersion = frameworkVersion
		plan.GoVersion = defaultGoVersion

		return nil
	}

	fwDir := os.Getenv(zeverFrameworkPathEnv)

	if fwDir == "" {
		detected, err := detectFrameworkCheckout()
		if err != nil {
			return fmt.Errorf("%s: %w", tag, err)
		}

		fwDir = detected
	}

	if fwDir == "" {
		// No explicit path and no checkout detected: default to the
		// published module version with no replace directive.
		plan.FrameworkDir = ""
		plan.FrameworkVersion = defaultFrameworkVersion
		plan.GoVersion = defaultGoVersion

		return nil
	}

	abs, err := filepath.Abs(fwDir)
	if err != nil {
		return fmt.Errorf("%s: %w", tag, err)
	}

	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		if _, werr := os.Stat(filepath.Join(abs, "go.work")); werr != nil {
			return fmt.Errorf("%s: %q does not look like a zever checkout (no go.mod or go.work): %w", tag, fwDir, err)
		}
	}

	plan.FrameworkDir = abs
	plan.FrameworkVersion = pseudoVersionZero
	plan.GoVersion = detectGoVersion(abs)

	return nil
}

// detectFrameworkCheckout walks up from the working directory looking for a
// go.mod declaring "module github.com/zenta-dev/zever" or a go.work
// workspace file -- the same clone this command itself is most likely being
// run from during framework development, since there is no tagged release
// to depend on any other way.
// Returns "" (not an error) when neither is found before the filesystem root.
func detectFrameworkCheckout() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}

	for {
		if modulePathOf(filepath.Join(dir, "go.mod")) == frameworkModulePath {
			return dir, nil
		}

		if _, werr := os.Stat(filepath.Join(dir, "go.work")); werr == nil {
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
	"cdn":           "noop",
	"crypto":        "local",
	"db":            "sqlite",
	"document":      "local",
	"eventbus":      "memory",
	"flag":          "static",
	"geo":           "static",
	"i18n":          "embed",
	"idempotency":   "memory",
	"lock":          "memory",
	"log":           "slog",
	"mailer":        "log",
	"media":         "local",
	"notification":  "log",
	"observability": "stdout",
	"outbox":        "db",
	"password":      "argon2id",
	"payment":       "stub",
	"permission":    "noop",
	"queue":         "memory",
	"ratelimit":     "memory",
	"resilience":    "memory",
	"router":        "stdhttp",
	"scheduler":     "embedded",
	"search":        "db",
	"secrets":       "env",
	"session":       "memory",
	"storage":       "local",
	"tenant":        "single",
	"vectorstore":   "db",
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
// applying cfg.DBAdapter/CacheAdapter/QueueAdapter overrides and the
// general cfg.Adapters map (--adapters flag / picker picks) -- the single
// source both renderZeverYaml and writeNewProject's app.go render use, so
// the two files can never pick different adapters for the same service.
// The three legacy fields win over the map when both name a battery.
func (c NewConfig) batterySelections() []batterySelection {
	overrideFor := map[string]string{}
	for b, a := range c.Adapters {
		overrideFor[b] = a
	}

	for battery, adapter := range map[string]string{"db": c.DBAdapter, "cache": c.CacheAdapter, "queue": c.QueueAdapter} {
		if adapter != "" {
			overrideFor[battery] = adapter
		}
	}

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

// zeverYamlSchemaModeline is prepended to the scaffolded zever.yaml so
// editors with the YAML Language Server extension get autocomplete and
// inline validation against the sibling zever.schema.json this scaffold
// also writes. It is a YAML comment: config.Load ignores it like any other
// comment line.
const zeverYamlSchemaModeline = "# yaml-language-server: $schema=./zever.schema.json\n"

// renderZeverYaml builds the always-emitted zever.yaml: exactly
// cfg.Batteries' adapter picks (never the full config.Default() service
// map -- see NewConfig.Batteries' own doc comment for why the set is
// minimal by default and developer-composed beyond that).
//
// Adaptation note: the "project" table the predecessor scaffold wrote alongside the batteries
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
		return nil, fmt.Errorf("zever new: marshal zever.yaml: %w", err)
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

	if writeErr := write("Dockerfile", []byte(renderNewDockerfile(cfg.GoVersion))); writeErr != nil {
		return nil, writeErr
	}

	if writeErr := write(".dockerignore", []byte(newDockerignore)); writeErr != nil {
		return nil, writeErr
	}

	if compose := renderNewCompose(cfg.batterySelections()); compose != "" {
		if writeErr := write("compose.yaml", []byte(compose)); writeErr != nil {
			return nil, writeErr
		}
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

	if writeErr := write("zever.schema.json", config.SchemaJSON); writeErr != nil {
		return nil, writeErr
	}

	yamlData, err := renderZeverYaml(cfg)
	if err != nil {
		return nil, err
	}

	if writeErr := write("zever.yaml", append([]byte(zeverYamlSchemaModeline), yamlData...)); writeErr != nil {
		return nil, writeErr
	}

	return written, nil
}

// sharedModuleDirs lists every shared helper module directory in the
// framework checkout. Scaffolded projects never require these directly
// (`go mod tidy` resolves them as transitives), but the local-checkout
// replace set covers them so module-graph commands resolve offline.
// Verified against the checkout by TestNestedModuleDirsMatchCheckout:
// add the directory here when adding a shared go.mod.
var sharedModuleDirs = []string{
	"shared/apperror",
	"shared/cas",
	"shared/codec",
	"shared/dbconn",
	"shared/endpoint",
	"shared/firebase",
	"shared/grpcclient",
	"shared/httpclient",
	"shared/kvstore",
	"shared/lrucache",
	"shared/mcpclient",
	"shared/msgspan",
	"shared/outboxbridge",
	"shared/prompt",
	"shared/providersclient",
	"shared/providersopt",
	"shared/redisclient",
	"shared/redisopt",
	"shared/registry",
	"shared/retry",
	"shared/s3opts",
	"shared/traceprop",
}

// checkout that a scaffolded project's module graph may need a go.mod
// for: one adapters/<battery>/<adapter> dir per adapter module (directly
// required selections plus transitive requirements). It is verified
// against the checkout by TestNestedModuleDirsMatchCheckout: add the
// directory here when adding an adapter go.mod, except under tools/
// (separate release line, never a scaffold dependency).
var nestedModuleDirs = []string{
	"adapters/ai/anthropic", "adapters/ai/gemini", "adapters/ai/ollama", "adapters/ai/openai",
	"adapters/analytics/log", "adapters/analytics/posthog",
	"adapters/auth/jwt", "adapters/auth/oidc", "adapters/auth/session",
	"adapters/billing/paddle", "adapters/billing/stripe", "adapters/billing/stub",
	"adapters/cache/db", "adapters/cache/memory", "adapters/cache/redis",
	"adapters/cdn/cloudflare", "adapters/cdn/noop",
	"adapters/crypto/kms", "adapters/crypto/local",
	"adapters/db/postgres", "adapters/db/sqlite",
	"adapters/document/latex", "adapters/document/local", "adapters/document/remote",
	"adapters/eventbus/memory", "adapters/eventbus/redis",
	"adapters/flag/firebase", "adapters/flag/static",
	"adapters/geo/google", "adapters/geo/osm", "adapters/geo/static",
	"adapters/i18n/embed", "adapters/i18n/remote",
	"adapters/idempotency/db", "adapters/idempotency/memory", "adapters/idempotency/redis",
	"adapters/lock/memory", "adapters/lock/redis",
	"adapters/log/noop", "adapters/log/pretty", "adapters/log/slog", "adapters/log/zerolog",
	"adapters/mailer/log", "adapters/mailer/smtp",
	"adapters/media/ffmpeg", "adapters/media/local", "adapters/media/s3",
	"adapters/notification/fcm", "adapters/notification/log", "adapters/notification/twilio",
	"adapters/observability/noop", "adapters/observability/otlp", "adapters/observability/stdout",
	"adapters/outbox/cdc", "adapters/outbox/db", "adapters/outbox/memory",
	"adapters/password/argon2",
	"adapters/payment/paddle", "adapters/payment/stripe", "adapters/payment/stub",
	"adapters/permission/casbin", "adapters/permission/noop", "adapters/permission/rbac",
	"adapters/queue/db", "adapters/queue/memory", "adapters/queue/redis",
	"adapters/ratelimit/memory", "adapters/ratelimit/redis",
	"adapters/resilience/inproc", "adapters/resilience/redis",
	"adapters/router/fiber", "adapters/router/stdhttp",
	"adapters/scheduler/embedded", "adapters/scheduler/postgres",
	"adapters/search/meilisearch", "adapters/search/db",
	"adapters/secrets/env", "adapters/secrets/vault",
	"adapters/session/cookie", "adapters/session/db", "adapters/session/memory", "adapters/session/redis",
	"adapters/storage/local", "adapters/storage/r2", "adapters/storage/s3",
	"adapters/tenant/header", "adapters/tenant/single",
	"adapters/vectorstore/db", "adapters/vectorstore/qdrant",
	"adapters/webhook/http", "adapters/webhook/queue",
	"adapters/workflow/memory", "adapters/workflow/db",
}

// coreModulePath returns the framework module path for one battery's core
// interface module (e.g. github.com/zenta-dev/zever/core/cache).
func coreModulePath(battery string) string {
	return frameworkModulePath + "/core/" + battery
}

// adapterModulePath returns the framework module path for one battery
// selection's adapter module (e.g.
// github.com/zenta-dev/zever/adapters/cache/memory).
func adapterModulePath(s batterySelection) string {
	return frameworkModulePath + "/adapters/" + adapterDirName(s)
}

// renderNewGoMod writes the new project's go.mod: a require of the
// published zever module plus one require each for container, config, one
// core/<battery> module per selected battery and one
// adapters/<battery>/<adapter> module per selection (see adapterBinding),
// all at cfg.FrameworkVersion, so `go mod tidy` resolves exactly the
// chosen adapters (transitives need no pins). When FrameworkDir is set
// (explicit --framework-path or auto-detected checkout) each require gets
// a local replace directive pointing into the checkout instead, plus
// version-less replaces for every other adapter and core module so
// module-graph commands (`go list -m all`) resolve offline without adding
// requirements for adapters the project did not choose.
func renderNewGoMod(tag string, cfg NewConfig) ([]byte, error) {
	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "module %s\n\ngo %s\n\n", cfg.ModulePath, cfg.GoVersion)

	// Collect the framework modules for the selection, sorted for
	// deterministic output. The root require is always first.
	corePaths := make([]string, 0, len(cfg.Batteries))
	adapterPaths := make([]string, 0, len(cfg.Batteries))
	seen := map[string]bool{}

	for _, s := range cfg.batterySelections() {
		if path := coreModulePath(s.Battery); !seen[path] {
			seen[path] = true
			corePaths = append(corePaths, path)
		}

		if path := adapterModulePath(s); !seen[path] {
			seen[path] = true
			adapterPaths = append(adapterPaths, path)
		}
	}

	sort.Strings(corePaths)
	sort.Strings(adapterPaths)

	direct := make([]string, 0, 6+len(corePaths)+len(adapterPaths))
	direct = append(direct,
		frameworkModulePath+"/container",
		frameworkModulePath+"/config",
		// The worker entrypoint always resolves the job dispatcher types,
		// so core/job ships with every scaffold, selected or not.
		frameworkModulePath+"/core/job",
		// The container always wires the agent loop and RAG engine over the
		// resolved backends, so core/agent and core/rag ship with every
		// scaffold, selected or not.
		frameworkModulePath+"/core/agent",
		frameworkModulePath+"/core/rag",
		// Generated server entrypoints always wire authz + middleware.
		frameworkModulePath+"/core/authz",
		frameworkModulePath+"/core/middleware",
	)
	direct = append(direct, corePaths...)
	direct = append(direct, adapterPaths...)

	if cfg.FrameworkDir != "" {
		_, _ = fmt.Fprintf(&b, "// Local zever checkout via replace directives (from --framework-path or\n"+
			"// auto-detected framework checkout); remove to depend on the published\n// modules instead.\n")

		for _, path := range direct {
			rel, err := relativeTo(cfg.OutDir, filepath.Join(cfg.FrameworkDir, strings.TrimPrefix(path, frameworkModulePath+"/")))
			if err != nil {
				return nil, fmt.Errorf("%s: locate the framework from %q: %w", tag, cfg.OutDir, err)
			}

			_, _ = fmt.Fprintf(&b, "replace %s => %s\n", path, rel)
		}

		_, _ = fmt.Fprint(&b, "\n")

		// Graph-only replaces for framework modules outside the selection:
		// `go mod tidy` never needs them, but `go list -m` traverses the
		// full module graph (container requires every core module for its
		// wiring). Version-less replaces keep those commands offline
		// without widening the project's requirements.
		rest := make([]string, 0, len(nestedModuleDirs)+len(sharedModuleDirs))
		for _, dir := range nestedModuleDirs {
			if path := frameworkModulePath + "/" + dir; !seen[path] {
				rest = append(rest, path)
			}
		}

		for _, dir := range sharedModuleDirs {
			rest = append(rest, frameworkModulePath+"/"+dir)
		}

		for battery := range allServiceAdapters {
			if path := coreModulePath(battery); !seen[path] {
				rest = append(rest, path)
			}
		}

		sort.Strings(rest)

		if len(rest) > 0 {
			_, _ = fmt.Fprintf(&b, "// Module-graph replaces for the remaining framework\n"+
				"// modules (not required by this project; present so `go list -m`\n"+
				"// and friends resolve offline against the local checkout).\n")
		}

		for _, path := range rest {
			rel, err := relativeTo(cfg.OutDir, filepath.Join(cfg.FrameworkDir, strings.TrimPrefix(path, frameworkModulePath+"/")))
			if err != nil {
				return nil, fmt.Errorf("%s: locate the framework from %q: %w", tag, cfg.OutDir, err)
			}

			_, _ = fmt.Fprintf(&b, "replace %s => %s\n", path, rel)
		}

		if len(rest) > 0 {
			_, _ = fmt.Fprint(&b, "\n")
		}
	}

	for _, path := range direct {
		_, _ = fmt.Fprintf(&b, "require %s %s\n", path, cfg.FrameworkVersion)
	}

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

// newDockerignore keeps build context small and secrets out of the image:
// local sqlite files, build output, VCS metadata and JS deps never ship.
const newDockerignore = `# Local sqlite databases created by ` + "`zever db migrate`" + `.
*.db

# Build output.
dist/

# VCS metadata.
.git/

# JS deps (docs tooling only, never part of the Go image).
node_modules/
`

// renderNewDockerfile returns the reference Dockerfile for a scaffolded
// project: multi-stage golang builder -> distroless nonroot runtime,
// HEALTHCHECK against the scaffolded /healthz endpoint, zever.yaml copied
// alongside the binary. goVersion selects the builder tag
// (golang:<ver>-bookworm); empty falls back to defaultGoVersion. The output
// carries no secrets and no absolute local paths.
func renderNewDockerfile(goVersion string) string {
	if goVersion == "" {
		goVersion = defaultGoVersion
	}

	return fmt.Sprintf(`# Scaffolded by `+"`zever new`"+`. Reference production image.
#
# Build with: docker build -t app .
# Run with: docker run --rm -p 8080:8080 -p 9090:9090 app
# Config via environment (never baked in): docker run -e DB_DSN=... app

FROM golang:%s-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM busybox:1.36.1-musl AS busybox

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=busybox /bin/wget /usr/bin/wget
COPY --from=builder /out/server /app/server
COPY zever.yaml ./zever.yaml
EXPOSE 8080 9090
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD ["/usr/bin/wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:8080/healthz"]
`, goVersion)
}

// renderNewCompose returns compose.yaml for a scaffolded project, or ""
// when no picked battery needs external infra. Postgres picks (db,
// search, vectorstore, workflow) add a db service; redis picks (cache, queue,
// session, ratelimit, lock, eventbus, idempotency) share one redis
// service. Env carries secrets at run time (never baked in); per-battery
// URL vars point at the compose services.
//
// The canonical "db" adapter name always provisions postgres: it selects
// the backend by DSN at runtime (empty means embedded sqlite), and the
// compose db service is what a postgres DSN points at. Legacy alias names
// ("postgres", "pgvector") provision it too; legacy "sqlite" never does.
func renderNewCompose(sel []batterySelection) string {
	needsPG := false
	redisBatteries := []string{}

	for _, s := range sel {
		switch {
		case s.Battery == "db" && s.Adapter == "postgres":
			needsPG = true
		case (s.Battery == "search" && (s.Adapter == "db" || s.Adapter == "postgres")) ||
			(s.Battery == "vectorstore" && (s.Adapter == "db" || s.Adapter == "pgvector")) ||
			(s.Battery == "workflow" && (s.Adapter == "db" || s.Adapter == "postgres")):
			needsPG = true
		case (s.Battery == "cache" || s.Battery == "queue" || s.Battery == "session" ||
			s.Battery == "ratelimit" || s.Battery == "lock" || s.Battery == "eventbus" ||
			s.Battery == "idempotency") && s.Adapter == "redis":
			redisBatteries = append(redisBatteries, s.Battery)
		}
	}

	if !needsPG && len(redisBatteries) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Scaffolded by `zever new`. External services for local prod-like runs.\n")
	b.WriteString("#\n")
	b.WriteString("# Up with: docker compose up --build\n")
	b.WriteString("# Secrets via environment (never baked in): DB_PASSWORD=... docker compose up --build\n")
	b.WriteString("#\n")
	b.WriteString("# Worker uses the same image:\n")
	b.WriteString("#   docker compose run --rm app ./worker\n")
	b.WriteString("services:\n")
	b.WriteString("  app:\n")
	b.WriteString("    build: .\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"8080:8080\"\n")
	b.WriteString("      - \"9090:9090\"\n")
	if needsPG || len(redisBatteries) > 0 {
		b.WriteString("    depends_on:\n")
		if needsPG {
			b.WriteString("      db:\n        condition: service_healthy\n")
		}
		if len(redisBatteries) > 0 {
			b.WriteString("      redis:\n        condition: service_healthy\n")
		}
	}
	b.WriteString("    environment:\n")
	if needsPG {
		b.WriteString("      DB_DSN: postgres://app:${DB_PASSWORD:-devsecret}@db:5432/app?sslmode=disable\n")
	}
	for _, battery := range redisBatteries {
		b.WriteString("      " + strings.ToUpper(battery) + "_URL: redis://redis:6379\n")
	}
	if needsPG {
		b.WriteString("  db:\n")
		b.WriteString("    image: postgres:16-bookworm\n")
		b.WriteString("    environment:\n")
		b.WriteString("      POSTGRES_USER: app\n")
		b.WriteString("      POSTGRES_PASSWORD: ${DB_PASSWORD:-devsecret}\n")
		b.WriteString("      POSTGRES_DB: app\n")
		b.WriteString("    volumes:\n")
		b.WriteString("      - pgdata:/var/lib/postgresql/data\n")
		b.WriteString("    healthcheck:\n")
		b.WriteString("      test: [\"CMD-SHELL\", \"pg_isready -U app\"]\n")
		b.WriteString("      interval: 5s\n")
		b.WriteString("      timeout: 3s\n")
		b.WriteString("      retries: 10\n")
	}
	if len(redisBatteries) > 0 {
		b.WriteString("  redis:\n")
		b.WriteString("    image: redis:7-alpine\n")
		b.WriteString("    volumes:\n")
		b.WriteString("      - redisdata:/data\n")
		b.WriteString("    healthcheck:\n")
		b.WriteString("      test: [\"CMD\", \"redis-cli\", \"ping\"]\n")
		b.WriteString("      interval: 5s\n")
		b.WriteString("      timeout: 3s\n")
		b.WriteString("      retries: 10\n")
	}
	b.WriteString("volumes:\n")
	if needsPG {
		b.WriteString("  pgdata:\n")
	}
	if len(redisBatteries) > 0 {
		b.WriteString("  redisdata:\n")
	}
	return b.String()
}

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

# turn schema/app.zen into a query builder (generated/zenorm/orm/gen/app), a
# Protobuf service definition (generated/proto) and SQL DDL
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

## Docker

` + "```bash" + `
docker build -t %[1]s .
docker run --rm -p 8080:8080 -p 9090:9090 %[1]s
` + "```" + `

Pass config at runtime via environment (never baked into the image),
e.g. ` + backtick + `docker run -e DB_DSN=... %[1]s` + backtick + `. The image
healthchecks ` + backtick + `GET /healthz` + backtick + `.

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

// printNewDryRun prints the scaffold plan without writing anything.
func printNewDryRun(cfg NewConfig, outDir string) {
	_, _ = fmt.Fprintln(os.Stdout, dim("dry run — no files written"))
	_, _ = fmt.Fprintln(os.Stdout, success("would scaffold ")+bold(fmt.Sprintf("%q", cfg.Name))+dim(" into ")+cyan(outDir))
	_, _ = fmt.Fprintln(os.Stdout, "  "+dim("module path: ")+cyan(cfg.ModulePath))

	if len(cfg.Batteries) > 0 {
		_, _ = fmt.Fprintln(os.Stdout, "  "+dim("batteries: ")+cyan(strings.Join(cfg.Batteries, ", ")))
	}
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
