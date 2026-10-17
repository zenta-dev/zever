package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zenta-dev/zever/config"
)

const newUsage = `zever new <name> [--module PATH] [--dir PATH] [--framework-version V] [--force]
                      [--with a,b=c] [--yes] [--list-batteries] [--no-tidy] [--dry-run]

Scaffolds a brand new, buildable zever application from scratch: a fresh
go.mod, a starter schema/app.zen, cmd/server/main.go,
internal/app/wiring.go and internal/service/hello, at exactly the default
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

What is NOT generated: real HTTP route handlers beyond the sample hello
endpoint, and real job logic. Every entrypoint this command writes has the
same TODO-stub ceiling as ` + "`zever generate server`" + `/` + "`worker`" + `/` + "`seed`" + `: this gets you to a buildable,
runnable skeleton in one command, not to a finished app.

Battery picker (--with): without --interactive this command never prompts
and scaffolds the minimal stack (db+log+router) plus whatever --with
selects. --with takes a comma-separated list of batteries with optional
adapter pins (e.g. --with cache,queue=redis). Pass --interactive for the
huh wizard (TTY required): a MultiSelect over every known battery (minimal
stack pre-selected), then one adapter Select per picked non-floor
battery. Every prompt answer has a flag: --with skips the battery prompt,
--with b=a skips that battery's adapter prompt, and -y/--yes skips the
whole wizard. --list-batteries prints the battery/default-adapter table
and exits. --profile is a deprecated alias: it prints the equivalent
--with form and then behaves as before.

Flags:`

// Framework wiring shared by `zever new` and `zever extract`.
const (
	// frameworkModulePath is zever's own module path. Scaffolds depend on
	// it: generated code imports github.com/zenta-dev/zever/... packages.
	frameworkModulePath = "github.com/zenta-dev/zever"

	// grpcGoToolModulePath is the Go tool dependency a scaffolded go.mod
	// needs so that "go tool protoc-gen-go-grpc" works out of the box.
	grpcGoToolModulePath = "google.golang.org/grpc/cmd/protoc-gen-go-grpc"

	// grpcGoToolVersion pins the tool dependency (matching dsl/go.mod) so
	// `go mod tidy` resolves offline from the module cache.
	grpcGoToolVersion = "v1.6.2"

	// defaultGoVersion is used when no go directive can be read from the
	// framework checkout or an existing project.
	defaultGoVersion = "1.27.2"

	// pseudoVersionZero is the placeholder version a require line carries
	// when the real resolution comes from a local replace directive.
	pseudoVersionZero = "v0.0.0-00010101000000-000000000000"
)

var (
	// FrameworkVersion is the published framework version a new project
	// depends on by default and `zever add` pins new requirements to. It
	// tracks the CLI release version (see buildVersion in version.go).
	FrameworkVersion = "v" + buildVersion

	// defaultFrameworkVersion is the published zever version a new project
	// depends on by default when neither --framework-version is given nor
	// a framework checkout is detected.
	defaultFrameworkVersion = FrameworkVersion
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
	// entries in the scaffolded service map. Empty means "use
	// config.Default()'s pick" (sqlite / memory / memory). Only ever
	// populated by the TUI wizard — the flag-driven path has no equivalent
	// flags and always leaves these empty.
	DBAdapter    string
	CacheAdapter string
	QueueAdapter string

	// Batteries is the full set of service names to import into
	// internal/app/app.go: always coreBatteries
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
	fs.String("batteries", "", "comma-separated extra batteries to scaffold (e.g. --batteries db,cache); prefer --with")
	fs.String("adapters", "", "comma-separated battery=adapter pairs (e.g. --adapters db=postgres,router=fiber); prefer --with")
	fs.String("with", "", "comma-separated batteries with optional adapter pins (e.g. --with cache,queue=redis); replaces --batteries/--adapters")
	fs.String("profile", "", "deprecated alias: prints the equivalent --with form, then behaves as before (minimal|api|full)")
	fs.Bool("yes", false, "skip all prompts, scaffold floor+flags only")
	fs.Bool("y", false, "alias for --yes")
	fs.Bool("list-batteries", false, "print the battery/default-adapter table to stdout and exit")
	fs.Bool("dry-run", false, "print the scaffold plan without writing any files")
	fs.Bool("no-tidy", false, "skip running 'go mod tidy' after scaffolding")

	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), newUsage+"\n", defaultFrameworkVersion)
		printFlagDefaults(fs)
	}

	return fs
}

// printNewUsage writes the full `zever new` help (header + real flag
// defaults) to fs.Output, for the legacy `zever help new` path.
func printNewUsage(fs *flag.FlagSet) {
	_, _ = fmt.Fprintf(fs.Output(), newUsage+"\n", defaultFrameworkVersion)
	printFlagDefaults(fs)
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

	// Flag-parity inputs: --with selects extra batteries with optional
	// adapter pins (e.g. --with cache,db=postgres), unifying the legacy
	// --batteries and --adapters flags, which keep working. --profile is a
	// deprecated alias that prints its --with replacement and behaves as
	// before; explicit --with/--batteries add to the preset.
	withBatteries, withAdapters, err := parseWithFlag(flagString("with"))
	if err != nil {
		return err
	}

	optional, err := parseBatteriesFlag(flagString("batteries"))
	if err != nil {
		return err
	}

	if len(withBatteries) > 0 {
		optional = mergeBatteries(optional, withBatteries)
	}

	if profile := flagString("profile"); profile != "" {
		_, _ = fmt.Fprintln(os.Stderr, formatHint(profileReplacementHint(profile)))

		profileBase, profileErr := profileBatteries(profile)
		if profileErr != nil {
			return profileErr
		}

		optional = mergeBatteries(optional, profileBase)
	}

	overrides, err := parseAdaptersFlag(flagString("adapters"))
	if err != nil {
		return err
	}

	for b, a := range withAdapters {
		overrides[b] = a
	}

	for b := range overrides {
		if !batteryPicked(optional, b) {
			optional = append(optional, b)
		}
	}

	// The picker is strictly opt-in: only --interactive (or the global
	// interactive mode / env) enters it, and -y/--yes always skips it.
	// Flag-supplied answers survive into the wizard: --with/--batteries
	// skips the battery prompt, --with b=a/--adapters skips that battery's
	// adapter prompt.
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

	if !flagBool(fs, "no-tidy") && !plan.ExistingProject {
		if tidyErr := runGoModTidyFunc(tag, outDir); tidyErr != nil {
			return tidyErr
		}
	}

	printNewSummary(plan, written)

	return nil
}

// DefaultTidyTimeout bounds `go mod tidy` after scaffolding.
const DefaultTidyTimeout = 2 * time.Minute

// runGoModTidyFunc is a seam so tests stub the toolchain.
var runGoModTidyFunc = runGoModTidy

// runGoModTidy runs `go mod tidy` in dir, returning an actionable error on
// failure instead of raw go output.
func runGoModTidy(tag, dir string) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("%s: go toolchain not found on PATH, install Go (https://go.dev/doc/install) then run `go mod tidy` in %q manually: %w", tag, dir, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTidyTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, goBin, "mod", "tidy")
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			trimmed = err.Error()
		}

		return fmt.Errorf("%s: go mod tidy in %q failed: %s: %w: %s", tag, dir, tidyFailureHint(string(out), dir), err, trimmed)
	}

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
			return nil, unknownBatteryError("zever new", name)
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

// parseWithFlag parses --with (comma-separated batteries with optional
// adapter pins, e.g. "cache,db=postgres") into a battery list plus an
// adapter override map. It unifies the legacy --batteries and --adapters
// grammars: a bare name selects a battery, name=adapter pins its adapter
// (and implies the battery). Unknown batteries and adapters fail closed
// with the choices listed. Empty means "no extras, no overrides".
func parseWithFlag(raw string) ([]string, map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, map[string]string{}, nil
	}

	set := map[string]bool{}
	overrides := map[string]string{}

	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		battery, adapter, hasAdapter := strings.Cut(part, "=")
		battery = strings.TrimSpace(battery)
		adapter = strings.TrimSpace(adapter)

		if battery == "" || (hasAdapter && adapter == "") {
			return nil, nil, fmt.Errorf("zever new: malformed --with entry %q, want battery or battery=adapter", part)
		}

		if _, ok := allServiceAdapters[battery]; !ok {
			return nil, nil, unknownBatteryError("zever new", battery)
		}

		set[battery] = true

		if !hasAdapter {
			continue
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
			return nil, nil, unknownAdapterError("zever new", battery, adapter, choices)
		}

		overrides[battery] = adapter
	}

	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}

	sort.Strings(out)

	return out, overrides, nil
}

// profileReplacementHint names the --with form replacing a deprecated
// --profile value, so the alias stays usable while steering callers off
// it. Unknown profiles still fail in profileBatteries; this only covers
// the message for known ones (and echoes the input otherwise).
func profileReplacementHint(profile string) string {
	extras, err := profileBatteries(profile)
	if err != nil {
		return fmt.Sprintf("--profile %q is deprecated, use --with instead (see --list-batteries)", profile)
	}

	if len(extras) == 0 {
		return "--profile minimal is deprecated, use --with (bare, for the minimal db+log+router stack) instead"
	}

	return fmt.Sprintf("--profile %q is deprecated, use --with %s instead", profile, strings.Join(extras, ","))
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
			return nil, unknownBatteryError("zever new", battery)
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
			return nil, unknownAdapterError("zever new", battery, adapter, choices)
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

	_, _ = fmt.Fprintln(os.Stdout, "")
	_, _ = fmt.Fprintln(os.Stdout, "PROFILE BATTERIES")
	_, _ = fmt.Fprintln(os.Stdout, "minimal (bootable floor only)")
	_, _ = fmt.Fprintln(os.Stdout, "api "+strings.Join(apiProfileBatteries, ","))
	_, _ = fmt.Fprintln(os.Stdout, "full (every battery)")
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

	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil { //nolint:gosec // fwDir is an explicit user flag; read-only checkout probe, no write or serve
		if _, werr := os.Stat(filepath.Join(abs, "go.work")); werr != nil { //nolint:gosec // fwDir is an explicit user flag; read-only checkout probe, no write or serve
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

// minimalScaffoldBatteries is the fixed, always-scaffolded battery set: the
// minimal stack every generated server boots. db (sqlite) persists the
// sample entity, log (slog) is the stdlib logger, router (stdhttp) is the
// net/http router, observability (stdout) backs serve's tracing middleware
// and ratelimit (memory) backs its rate-limit middleware -- serve resolves
// all five unconditionally at boot, so omitting any of them fails `zever
// serve` with an unknown-adapter error. Anything beyond this is added via
// `zever new --with` (or `zever add` later, or with service env plus one
// Provider entry in wiring.go).
var minimalScaffoldBatteries = []string{
	"db", "log", "observability", "ratelimit", "router",
}

// apiProfileBatteries are the extras the "api" start-small profile adds on
// top of the boot set: the batteries a JSON API with background jobs needs,
// all on zero-infra default adapters (memory/embedded/db) so the scaffold
// still boots offline. Sorted; profileBatteries clones it.
var apiProfileBatteries = []string{
	"cache",
	"idempotency",
	"lock",
	"outbox",
	"password",
	"queue",
	"scheduler",
	"session",
}

// fullProfileSkipped are the services the "full" profile leaves out: none has
// an adapter that resolves from env defaults alone (ai needs an API key, geo a
// dataset file, i18n an embedded FS), so wiring them would scaffold a project
// that fails `zever doctor`. Add them with `zever add <battery>`.
var fullProfileSkipped = map[string]bool{"ai": true, "geo": true, "i18n": true}

// profileBatteries resolves a --profile name to its extra batteries (on top
// of the minimal stack batteriesFor always includes). "" means no profile
// was passed (nil extras, no behavior change); "minimal" is the minimal
// stack alone; "api" adds apiProfileBatteries; "full" covers every known
// service. Unknown names fail closed listing the choices. Results are
// sorted and freshly allocated, safe for callers to merge and sort
// further. --profile is deprecated (see profileReplacementHint); --with
// takes the same battery lists directly.
func profileBatteries(profile string) ([]string, error) {
	switch profile {
	case "":
		return nil, nil
	case "minimal":
		return []string{}, nil
	case "api":
		return append([]string(nil), apiProfileBatteries...), nil
	case "full":
		boot := map[string]bool{}
		for _, b := range batteriesFor(nil, "") {
			boot[b] = true
		}

		extras := make([]string, 0, len(allServiceAdapters))
		for battery := range allServiceAdapters {
			if !boot[battery] && !fullProfileSkipped[battery] {
				extras = append(extras, battery)
			}
		}

		sort.Strings(extras)

		return extras, nil
	default:
		return nil, fmt.Errorf("zever new: unknown --profile %q (choices: minimal, api, full)", profile)
	}
}

// batteriesFor merges minimalScaffoldBatteries with optional
// (developer-picked) batteries, adding "cache" whenever cacheAdapter is a
// real override -- explicit intent to use caching even when "cache" wasn't
// itself picked.
func batteriesFor(optional []string, cacheAdapter string) []string {
	batteries := mergeBatteries(minimalScaffoldBatteries, optional)

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
// source writeNewProject's app.go render uses for adapter wiring.
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

// ensureTargetDir refuses to scaffold into a non-empty existing directory
// unless force is set, mirroring the collision-protection convention
// writeScaffold already applies per-file. A directory that does not exist
// yet is created.
func ensureTargetDir(tag, dir string, force bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			if mkdirErr := os.MkdirAll(dir, 0o750); mkdirErr != nil { // dir is the CLI's own --dir/positional arg, developer-supplied
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
// here, so every scaffold writes the same minimal tree: go.mod,
// schema/app.zen, cmd/server/main.go, internal/app/wiring.go,
// internal/service/hello, .env, .env.example, .gitignore and README.md.
// Heavier files (worker, seed, Docker, CI, lint, compose, k8s) are opt-in
// `zever add` recipes, not defaults.
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

	selections := cfg.batterySelections()

	wiringContent, err := renderWiringContent(tag, cfg.ModulePath, selections)
	if err != nil {
		return nil, err
	}

	if writeErr := write(filepath.Join("internal", "app", "wiring.go"), wiringContent); writeErr != nil {
		return nil, writeErr
	}

	serverContent, err := renderGoFile(tag, "server main.go", newServerMainTemplate, struct{ ModulePath string }{cfg.ModulePath})
	if err != nil {
		return nil, err
	}

	if writeErr := write(filepath.Join(defaultServerEntry, "main.go"), serverContent); writeErr != nil {
		return nil, writeErr
	}

	helloContent, err := renderGoFile(tag, "hello service", helloServiceTemplate, struct{ ModulePath string }{cfg.ModulePath})
	if err != nil {
		return nil, err
	}

	if writeErr := write(filepath.Join("internal", "service", "hello", "service.go"), helloContent); writeErr != nil {
		return nil, writeErr
	}

	devEnv, err := renderDevEnv(selections)
	if err != nil {
		return nil, err
	}

	if writeErr := write(".env", devEnv); writeErr != nil {
		return nil, writeErr
	}

	if writeErr := write(".env.example", renderEnvExample(selections)); writeErr != nil {
		return nil, writeErr
	}

	if writeErr := write(".gitignore", []byte(newGitignore)); writeErr != nil {
		return nil, writeErr
	}

	if writeErr := write("README.md", []byte(renderNewReadme(cfg.Name))); writeErr != nil {
		return nil, writeErr
	}

	// The sqlite default DSN (data/app.db, see the quickstart) needs its
	// directory to exist: keeping it in the tree means the scaffolded
	// binaries and `zever db migrate` run without a mkdir first.
	if writeErr := write(filepath.Join("data", ".gitkeep"), nil); writeErr != nil {
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
	"shared/ref",
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
		// serve.Main boots the generated server from the typed Providers
		// plus the project Modules; every scaffold wires through it.
		frameworkModulePath+"/serve",
		// The default zenorm backend emits packages importing the orm
		// module, so every scaffold that compiles entities requires it
		// (pinned by TestNewScaffoldGoModRequiresOrm).
		frameworkModulePath+"/orm",
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
		rest := make([]string, 0, len(nestedModuleDirs)+len(sharedModuleDirs)+1)
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

	// Published mode: released framework go.mod files may require older
	// sibling modules than cfg.FrameworkVersion, and minimal version
	// selection would keep the older sibling (e.g. shared/registry lacking
	// symbols newer core modules call). Pin every other framework module at
	// the same version so the whole graph resolves consistently.
	if cfg.FrameworkDir == "" {
		pinned := map[string]bool{}
		for _, path := range direct {
			pinned[path] = true
		}

		var extra []string

		add := func(dir string) {
			if path := frameworkModulePath + "/" + dir; !pinned[path] {
				pinned[path] = true
				extra = append(extra, path)
			}
		}

		for _, dir := range sharedModuleDirs {
			add(dir)
		}

		for battery := range allServiceAdapters {
			add("core/" + battery)
		}

		// core/eval is not a service battery but container's graph needs it.
		add("core/eval")

		// The server and worker entrypoints always wire the agent loop and
		// RAG engine types, authz policies, middleware and the job
		// dispatcher: pin them with the release even though the minimal
		// scaffold does not import them directly.
		for _, extra := range []string{"core/agent", "core/authz", "core/job", "core/middleware", "core/rag"} {
			add(extra)
		}

		for _, dir := range nestedModuleDirs {
			add(dir)
		}

		sort.Strings(extra)

		if len(extra) > 0 {
			_, _ = fmt.Fprint(&b, "\n// Version pins keeping every transitive framework module at the same\n"+
				"// release as the direct requirements; `go mod tidy` drops unused ones.\n")

			for _, path := range extra {
				_, _ = fmt.Fprintf(&b, "require %s %s // indirect\n", path, cfg.FrameworkVersion)
			}

			_, _ = fmt.Fprint(&b, "\n")
		}
	}

	// Pin the tool dependency version (matching dsl/go.mod) so `go mod tidy`
	// resolves offline from the module cache instead of querying the proxy
	// for the latest version providing the tool package.
	_, _ = fmt.Fprintf(&b, "require %s %s\n", grpcGoToolModulePath, grpcGoToolVersion)

	_, _ = fmt.Fprintf(&b, "\n// protogogen's grpc codegen shells out to this pinned tool dependency\n"+
		"// (\"go tool protoc-gen-go-grpc\") instead of requiring a protoc-gen-go-grpc\n"+
		"// binary on PATH; `go mod tidy` resolves its go.sum entries\n"+
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

// newGitignore keeps secrets and local state out of git: the dev .env file
// (random per project, never commit it), the local sqlite database, and
// the generated-code directory a fresh project has no reason to check in.
const newGitignore = `# Local dev secrets. Generated per project by ` + "`zever new`" + `; never commit.
.env
.env.local

# The local sqlite database is created at runtime; never commit it.
*.db

# Regenerated from schema/*.zen by ` + "`zever compile`" + ` -- see README.
/generated/
`

// renderNewReadme is a genuine quickstart, not filler: exactly four
// commands, each one a freshly scaffolded project can actually run, in
// order.
func renderNewReadme(name string) string {
	const backtick = "`"

	tmpl := `# %[1]s

A zever application, scaffolded by ` + backtick + `zever new` + backtick + `. One ` + backtick + `.zen` + backtick + ` schema
(` + backtick + `schema/app.zen` + backtick + `) is the source of truth; the ` + backtick + `zever` + backtick + ` CLI turns it into a
query builder, a Protobuf service definition and the SQL schema, and
everything else is resolved from a ` + backtick + `container.Container` + backtick + ` built in
` + backtick + `internal/app/wiring.go` + backtick + `.

## Quickstart

` + "```bash" + `
# turn schema/app.zen into a query builder, a Protobuf service definition
# and SQL DDL
zever compile --backend=zenorm,proto,atlas schema/app.zen

# create the sqlite database from the schema (safe to re-run)
zever db migrate --adapter=sqlite --dsn=app.db schema/app.zen

# run the HTTP API, then try the sample endpoint
zever serve
curl localhost:8080/hello
` + "```" + `

## What's here, and what's not

- ` + backtick + `schema/app.zen` + backtick + ` -- one starter ` + backtick + `User` + backtick + ` entity. Extend it with
  ` + backtick + `zever generate entity/job/schedule` + backtick + `.
- ` + backtick + `cmd/server/main.go` + backtick + ` -- the thin entrypoint; it calls
  ` + backtick + `serve.Main` + backtick + ` with the providers and modules below.
- ` + backtick + `internal/app/wiring.go` + backtick + ` -- the one place this project selects its
  adapters: one ` + backtick + `Ref` + backtick + ` var plus one ` + backtick + `container.Provide` + backtick + ` entry per
  service instance. Add batteries later with ` + backtick + `zever add <battery>` + backtick + `, which only
  inserts entries here and never rewrites your code.
- ` + backtick + `internal/service/hello/service.go` + backtick + ` -- the sample module mounted
  by ` + backtick + `wiring.Modules` + backtick + ` (` + backtick + `GET /hello` + backtick + `). Replace it with real
  modules as the app grows.
- ` + backtick + `.env` + backtick + ` / ` + backtick + `.env.example` + backtick + ` -- local config. ` + backtick + `.env` + backtick + `
  holds per-project dev secrets (never commit it); ` + backtick + `.env.example` + backtick + `
  documents every key (generated from the adapter Options structs).

Need a worker, seed, Dockerfile, CI, lint, compose or k8s manifests?
` + backtick + `zever add worker|seed|docker|ci|lint|compose|k8s` + backtick + ` writes them from
templates when you need them -- the default scaffold stays minimal on
purpose.
`

	return fmt.Sprintf(tmpl, name)
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
		"  "+dim("2.")+"  "+cmd("zever serve"),
		"  "+dim("3.")+"  "+cmd("curl localhost:8080/hello"),
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

// --- typed wiring scaffold (wiring.go, server main, hello, env files) ---

// wiringTemplateData is wiringTemplate's render input: RefVars declares one
// Ref var per wired instance, Providers holds one container.Provide entry
// per instance, and the import lists pin exactly the used modules.
type wiringTemplateData struct {
	ModulePath     string
	AdapterImports []moduleImport
	CoreImports    []string
	RefVars        []wiringRefVar
	Providers      []string
}

// wiringRefVar is one Ref var declaration: Name is the exported var name
// (DB, DBAnalytics), Doc its one-line comment, Expr the initializer
// (db.DefaultRef(), db.Ref("analytics")).
type wiringRefVar struct {
	Name string
	Doc  string
	Expr string
}

// noErrAdapters names adapter dirs whose New takes Options but returns only
// the service (no error): the wiring wraps them in container.WrapNoErr.
// Verified against the checkout; add the dir here when an adapter gains or
// loses an error return.
var noErrAdapters = map[string]bool{
	"log/slog":    true,
	"log/zerolog": true,
	"log/pretty":  true,
}

// noOptsAdapters names adapter dirs whose New takes no arguments at all:
// the wiring wraps them in container.WrapNoOpts.
var noOptsAdapters = map[string]bool{
	"log/noop":           true,
	"observability/noop": true,
	"billing/stub":       true,
}

// embeddedDBAdapters names adapter dirs whose New takes an adapter-local
// Options embedding the battery Options (as field Options): the wiring
// closes over the core Options and nests them. Verified against the
// checkout; every entry's Options struct embeds the core one.
var embeddedDBAdapters = map[string]bool{
	"cache/db":           true,
	"session/db":         true,
	"outbox/db":          true,
	"outbox/cdc":         true,
	"queue/db":           true,
	"idempotency/db":     true,
	"search/db":          true,
	"workflow/db":        true,
	"scheduler/postgres": true,
	"resilience/redis":   true,
}

// coreServiceTypes maps batteries with embedded-DB or mapped adapters to
// their core interface and Options type names, used in the generated
// factory closure signatures. Plain (core-Options) adapters need no entry:
// type inference reads them off the adapter New.
var coreServiceTypes = map[string][2]string{
	"cache":       {"Cache", "Options"},
	"session":     {"Store", "Options"},
	"outbox":      {"Store", "Options"},
	"queue":       {"Queue", "Options"},
	"idempotency": {"Store", "Options"},
	"search":      {"Search", "Options"},
	"workflow":    {"Workflow", "Options"},
	"scheduler":   {"Scheduler", "Options"},
	"resilience":  {"Manager", "Options"},
	"secrets":     {"Secrets", "Options"},
}

// mappedAdapters names adapter dirs whose local Options cannot embed the
// core Options but map field-for-field: the wiring builds them explicitly.
// The mapped field list is per adapter, verified against the checkout.
var mappedAdapters = map[string]map[string]string{
	"secrets/env": {"Prefix": "o.Prefix"},
}

// blockedAdapters names adapter dirs with no standalone provider factory:
// wiring them needs hand-written code, so `zever add` fails closed with
// the documented alternative instead of emitting code that cannot compile.
var blockedAdapters = map[string]string{
	"session/cookie": "the cookie adapter is a response helper, not a session store; pick memory, redis or db instead",
	"secrets/vault":  "the vault adapter needs hand-wired credentials; wire it in wiring.go with container.Provide and secrets Options",
	"crypto/kms":     "the kms adapter needs hand-wired key material; wire it in wiring.go with container.Provide and kms Options",
}

// factoryExpr returns the container.Provide factory expression for one
// battery selection, or an error for blocked adapters. alias is the
// adapter import alias (see adapterBinding), dir the adapter module dir.
// ind indents the entry line: multi-line closures hang their body one
// level deeper and close at ind.
//
//nolint:unparam // every generated entry sits at two tabs today; ind documents the invariant for hand-indented files.
func factoryExpr(sel batterySelection, alias, ind string) (string, error) {
	dir := adapterDirName(sel)

	if reason, blocked := blockedAdapters[dir]; blocked {
		return "", fmt.Errorf("zever add: battery %q adapter %q cannot be auto-wired: %s", sel.Battery, sel.Adapter, reason)
	}

	if noOptsAdapters[dir] {
		return "container.WrapNoOpts(" + alias + ".New)", nil
	}

	if noErrAdapters[dir] {
		return "container.WrapNoErr(" + alias + ".New)", nil
	}

	if fields, mapped := mappedAdapters[dir]; mapped {
		types, ok := coreServiceTypes[sel.Battery]
		if !ok {
			return "", fmt.Errorf("zever add: battery %q adapter %q has no mapped factory", sel.Battery, sel.Adapter)
		}

		sets := make([]string, 0, len(fields))
		for field, src := range fields {
			sets = append(sets, field+": "+src)
		}

		sort.Strings(sets)

		return fmt.Sprintf("func(o %s.Options) (%s.%s, error) {\n%s\treturn %s.New(%s.Options{%s})\n%s}",
			sel.Battery, sel.Battery, types[0], ind, alias, alias, strings.Join(sets, ", "), ind), nil
	}

	if embeddedDBAdapters[dir] {
		types, ok := coreServiceTypes[sel.Battery]
		if !ok {
			return "", fmt.Errorf("zever add: battery %q adapter %q has no embedded factory", sel.Battery, sel.Adapter)
		}

		return fmt.Sprintf("func(o %s.Options) (%s.%s, error) {\n%s\treturn %s.New(%s.Options{Options: o})\n%s}",
			sel.Battery, sel.Battery, types[0], ind, alias, alias, ind), nil
	}

	return alias + ".New", nil
}

// refVarName returns the exported Ref var name for a battery instance:
// the title-cased battery for the default instance (DB, Cache, RateLimit),
// plus the title-cased instance name for named ones (DBAnalytics).
// Well-known initialisms and compounds match config.Config's field names
// (DB, RateLimit); anything else uppercases the first letter only.
func refVarName(battery, name string) string {
	capitalize := func(s string) string {
		if s == "" {
			return s
		}

		return strings.ToUpper(s[:1]) + s[1:]
	}

	base := capitalize(battery)
	if canon, ok := map[string]string{"db": "DB", "ratelimit": "RateLimit"}[battery]; ok {
		base = canon
	}

	if name == "" || name == "default" {
		return base
	}

	inst := capitalize(name)
	if inst == "Db" {
		inst = "DB"
	}

	return base + inst
}

// wiringTemplate is the one place a project selects its adapters: one Ref
// var plus one container.Provide entry per service instance, all explicit
// so the file stays greppable and `zever add` can edit it with go/ast.
// Adapter packages expose constructors but never self-register, so wiring
// is an explicit import plus a Provide entry; config comes from the
// environment at boot (see serve.Main). This file is scaffolded once and
// then user-owned: `zever add` only inserts Provider entries idempotently
// and never rewrites user code (see R1).
const wiringTemplate = `// Package app wires this project's service instances.
//
// The Ref vars below name each wired instance; Providers builds them from
// the named adapter constructors; Modules mounts the project's serve
// modules. Add batteries with ` + "`zever add <battery>`" + `: it only inserts Ref
// vars and Provider entries here and never rewrites your code. Tune an
// instance with container.WithOptions entries in Providers.
package app

import (
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/serve"
{{range .CoreImports}}	"{{.}}"
{{end}}{{range .AdapterImports}}	{{.Alias}} {{printf "%q" .Path}}
{{end}}
	"{{.ModulePath}}/internal/service/hello"
)
{{range .RefVars}}
// {{.Name}} {{.Doc}}
var {{.Name}} = {{.Expr}}{{end}}

// Providers returns every wired service instance, in declaration order.
// The container validates the graph (duplicate refs, missing deps,
// cycles) and resolves each instance lazily on first use.
func Providers() []container.Provider {
	return []container.Provider{
{{range .Providers}}		{{.}},
{{end}}	}
}

// Modules returns the serve modules mounted at boot, after the shared
// gRPC server is built: each module registers its routes (and gRPC
// services) on the resolved router via Mount.
func Modules() func(*container.Container) []serve.Module {
	return func(*container.Container) []serve.Module {
		return []serve.Module{
			hello.New(),
		}
	}
}
`

// renderWiringContent renders wiringTemplate for exactly the given battery
// selections (floor + picks, nothing more): one named adapter import plus
// Provide entry per selection, one Ref var per instance, sorted
// deterministically.
func renderWiringContent(tag, modulePath string, selections []batterySelection) ([]byte, error) {
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
	refVars := make([]wiringRefVar, 0, len(sorted))
	providers := make([]string, 0, len(sorted))

	for _, s := range sorted {
		alias, path, _ := adapterBinding(s)
		adapterImports = append(adapterImports, moduleImport{Alias: alias, Path: path})

		if core := coreModulePath(s.Battery); !coreSeen[core] {
			coreSeen[core] = true
			coreImports = append(coreImports, core)
		}

		varName := refVarName(s.Battery, "default")
		refVars = append(refVars, wiringRefVar{
			Name: varName,
			Doc:  fmt.Sprintf("names the default %s instance (%s adapter).", s.Battery, s.Adapter),
			Expr: s.Battery + ".DefaultRef()",
		})

		factory, err := factoryExpr(s, alias, "\t\t")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", tag, err)
		}

		providers = append(providers, "container.Provide("+varName+", "+factory+")")
	}

	sort.Slice(adapterImports, func(i, j int) bool { return adapterImports[i].Path < adapterImports[j].Path })
	sort.Strings(coreImports)

	return renderGoFile(tag, "wiring.go", wiringTemplate, wiringTemplateData{
		ModulePath:     modulePath,
		AdapterImports: adapterImports,
		CoreImports:    coreImports,
		RefVars:        refVars,
		Providers:      providers,
	})
}

// newServerMainTemplate is the whole server entrypoint: parse nothing,
// wire nothing, just hand the typed Providers and Modules to serve.Main.
// serve returns the error; main owns the exit path (stderr + non-zero).
const newServerMainTemplate = `// Command server runs this project's HTTP API.
//
// Scaffolded by ` + "`zever new`" + `. It is deliberately thin: serve.Main owns
// boot, signal handling and graceful shutdown; everything with behaviour
// worth testing belongs in a package of its own.
package main

import (
	"fmt"
	"os"

	"github.com/zenta-dev/zever/serve"

	"{{.ModulePath}}/internal/app"
)

func main() {
	if err := serve.Main(app.Providers(), app.Modules()); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}
`

// helloServiceTemplate is the sample module mounted by wiring.Modules: a
// real GET /hello endpoint (not a TODO stub) proving the router, log and
// container wiring resolve end to end. Replace it with real modules as the
// app grows.
const helloServiceTemplate = `// Package hello is the sample module mounted by wiring.Modules: a real
// GET /hello endpoint proving the router, log and container wiring resolve
// end to end. Replace it with real modules as the app grows.
package hello

import (
	"encoding/json"
	"net/http"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/router"
)

// Service is the sample module: it carries no state, resolving the logger
// per request from the container so tests can swap providers freely.
type Service struct{}

// New builds the sample module.
func New() *Service {
	return &Service{}
}

// Name identifies the module in serve boot errors and logs.
func (s *Service) Name() string {
	return "hello"
}

// Mount registers GET /hello on r, resolving the logger from c.
func (s *Service) Mount(r router.Router, c *container.Container) error {
	var logger log.Logger

	logger, err := c.Log()
	if err != nil {
		return err
	}

	r.Handle(http.MethodGet, "/hello", func(w http.ResponseWriter, req *http.Request) {
		logger.Info().Str("path", req.URL.Path).Msg("hello")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	})

	return nil
}
`

// envKeyName maps a struct field name to its env key segment: upper-cased,
// matching the canonical <SERVICE>_<FIELD> form the README documents
// (DB_MAXCONNS, AUTH_JWT_SECRET). ApplyEnv resolves the same field through
// either spelling, so generated keys always round-trip.
func envKeyName(field string) string {
	return strings.ToUpper(field)
}

// envScalarValue formats one Options scalar for .env.example: strings
// verbatim, bools and integers plainly, durations in "5s" form. It mirrors
// ApplyEnv's coercions so every emitted value re-parses.
func envScalarValue(v reflect.Value) (string, bool) {
	if v.Type() == reflect.TypeOf(time.Duration(0)) {
		if d, ok := v.Interface().(time.Duration); ok {
			return d.String(), true
		}

		return "", false
	}

	switch kind := v.Kind(); kind { //nolint:exhaustive // default rejects every kind the env generator doesn't emit
	case reflect.String:
		return v.String(), true
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), true
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64), true
	default:
		return "", false
	}
}

// addressableCopy returns an addressable copy of the struct value in opts,
// so embedded fields can be addressed for recursion. Non-structs yield an
// invalid Value.
func addressableCopy(opts any) reflect.Value {
	v := reflect.Indirect(reflect.ValueOf(opts))
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return reflect.Value{}
	}

	cp := reflect.New(v.Type())
	cp.Elem().Set(v)

	return cp.Elem()
}

// appendEnvKeys walks one Options struct value into out as KEY=DEFAULT
// lines under prefix+sep: flat scalars directly, one nesting level as
// PREFIX_OUTER_INNER (mirroring ApplyEnv), skipping json:"-" fields and
// types with no env string form. sep is "_" for default instances and "__"
// for named ones (SERVICE__NAME__FIELD).
func appendEnvKeys(out *strings.Builder, prefix, sep string, opts any) {
	v := addressableCopy(opts)
	if !v.IsValid() {
		return
	}

	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}

		tag := sf.Tag.Get("json")
		if tag == "-" {
			continue
		}

		f := v.Field(i)

		if sf.Anonymous && f.Kind() == reflect.Struct {
			appendEnvKeys(out, prefix, sep, f.Addr().Interface())

			continue
		}

		if f.Kind() == reflect.Struct && f.Type() != reflect.TypeOf(time.Duration(0)) {
			nested := f
			for j := 0; j < nested.Type().NumField(); j++ {
				nsf := nested.Type().Field(j)
				if nsf.PkgPath != "" || nsf.Tag.Get("json") == "-" {
					continue
				}

				nf := nested.Field(j)

				val, ok := envScalarValue(nf)
				if !ok {
					continue
				}

				_, _ = fmt.Fprintf(out, "%s%s%s%s%s=%s\n", prefix, sep, envKeyName(sf.Name), sep, envKeyName(nsf.Name), val)
			}

			continue
		}

		val, ok := envScalarValue(f)
		if !ok {
			continue
		}

		_, _ = fmt.Fprintf(out, "%s%s%s=%s\n", prefix, sep, envKeyName(sf.Name), val)
	}
}

// renderEnvExample generates .env.example from the Options structs of the
// wired batteries: every key, default and type comment, straight from
// config.DefaultOptions, so the file cannot drift from what ApplyEnv
// resolves. Copy it to .env and fill secrets for local runs.
func renderEnvExample(selections []batterySelection) []byte {
	var b strings.Builder

	b.WriteString("# Managed by `zever new` and `zever add`. Copy to .env and fill secrets.\n")
	b.WriteString("# Keys are <SERVICE>_<FIELD> (default instance) or <SERVICE>__<NAME>__<FIELD> (named).\n")
	b.WriteString("# Defaults shown; every value re-parses via ApplyEnv. Adapter extras\n")
	b.WriteString("# (e.g. CACHE_TABLE) follow the same SERVICE_FIELD rule; tune the rest\n")
	b.WriteString("# with container.WithOptions in internal/app/wiring.go.\n")

	sorted := append([]batterySelection(nil), selections...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Battery != sorted[j].Battery {
			return sorted[i].Battery < sorted[j].Battery
		}

		return sorted[i].Adapter < sorted[j].Adapter
	})

	for _, s := range sorted {
		fmt.Fprintf(&b, "\n# --- %s (adapter %s) ---\n", s.Battery, s.Adapter)
		fmt.Fprintf(&b, "%s_ADAPTER=%s\n", strings.ToUpper(s.Battery), s.Adapter)

		if opts := config.DefaultOptions(s.Battery, ""); opts != nil {
			appendEnvKeys(&b, strings.ToUpper(s.Battery), "_", opts)
		}
	}

	return []byte(b.String())
}

// isSecretField reports whether a struct field carries secret material,
// via the framework's own `zever:"secret"` marker tag. Name heuristics
// would drift; the tag is the contract redact.go and friends already honor.
func isSecretField(sf reflect.StructField) bool {
	return sf.Tag.Get("zever") == "secret"
}

// collectDevSecrets walks one Options struct value, returning KEY=random
// lines for empty string fields tagged `zever:"secret"` under prefix (one
// nesting level, mirroring appendEnvKeys). Non-empty secret fields already
// carry a default and need no generated value; non-string secrets have no
// text form and are skipped.
func collectDevSecrets(out *strings.Builder, prefix string, opts any) {
	v := addressableCopy(opts)
	if !v.IsValid() {
		return
	}

	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" || sf.Tag.Get("json") == "-" {
			continue
		}

		f := v.Field(i)

		if sf.Anonymous && f.Kind() == reflect.Struct {
			collectDevSecrets(out, prefix, f.Addr().Interface())

			continue
		}

		if f.Kind() == reflect.Struct && f.Type() != reflect.TypeOf(time.Duration(0)) {
			nested := f
			for j := 0; j < nested.Type().NumField(); j++ {
				nsf := nested.Type().Field(j)
				if nsf.PkgPath != "" || nsf.Tag.Get("json") == "-" {
					continue
				}

				if !isSecretField(nsf) {
					continue
				}

				nf := nested.Field(j)
				if nf.Kind() != reflect.String || nf.String() != "" {
					continue
				}

				secret, err := randomSecret()
				if err != nil {
					continue
				}

				_, _ = fmt.Fprintf(out, "%s_%s_%s=%s\n", prefix, envKeyName(sf.Name), envKeyName(nsf.Name), secret)
			}

			continue
		}

		if !isSecretField(sf) {
			continue
		}

		if f.Kind() != reflect.String || f.String() != "" {
			continue
		}

		secret, err := randomSecret()
		if err != nil {
			continue
		}

		_, _ = fmt.Fprintf(out, "%s_%s=%s\n", prefix, envKeyName(sf.Name), secret)
	}
}

// randomSecret returns 32 random bytes as base64: the per-project dev
// secret shape for every `zever:"secret"` field and for the pre-generated
// AUTH_JWT_SECRET/CRYPTO_KEY. Failure (no OS entropy) is a hard error:
// secrets must never silently fall back to deterministic values.
func randomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("zever new: rand: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf), nil
}

// renderDevEnv generates .env: a random per-project CRYPTO_KEY (so serve's
// dev-key guard passes without ZEVER_ENV games), random dev secrets for
// every empty `zever:"secret"` field of the wired batteries, plus a
// pre-generated AUTH_JWT_SECRET so `zever add auth` works with zero extra
// steps. Dev only: rotate real values for production. Never commit this
// file.
func renderDevEnv(selections []batterySelection) ([]byte, error) {
	var b strings.Builder

	b.WriteString("# Local dev secrets. Generated per project by `zever new`; never commit.\n")
	b.WriteString("# Dev only: set real values via the environment in production.\n")

	cryptoKey, err := randomSecret()
	if err != nil {
		return nil, err
	}

	b.WriteString("\n# Random per project: serve boots without ZEVER_ENV games and no\n")
	b.WriteString("# two checkouts share key material. Production sets a real CRYPTO_KEY.\n")
	fmt.Fprintf(&b, "CRYPTO_KEY=%s\n", cryptoKey)

	sorted := append([]batterySelection(nil), selections...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Battery != "db" && sorted[j].Battery == "db" {
			return false
		}

		if sorted[i].Battery != sorted[j].Battery {
			return sorted[i].Battery < sorted[j].Battery
		}

		return sorted[i].Adapter < sorted[j].Adapter
	})

	for _, s := range sorted {
		if opts := config.DefaultOptions(s.Battery, ""); opts != nil {
			var section strings.Builder
			collectDevSecrets(&section, strings.ToUpper(s.Battery), opts)

			if section.Len() > 0 {
				fmt.Fprintf(&b, "\n# %s secrets\n", s.Battery)
				b.WriteString(section.String())
			}
		}
	}

	hasAuth := false

	for _, s := range sorted {
		if s.Battery == "auth" {
			hasAuth = true
		}
	}

	if !hasAuth {
		secret, err := randomSecret()
		if err != nil {
			return nil, err
		}

		b.WriteString("\n# Pre-generated so `zever add auth` works with zero extra steps.\n")
		fmt.Fprintf(&b, "AUTH_JWT_SECRET=%s\n", secret)
	}

	return []byte(b.String()), nil
}
