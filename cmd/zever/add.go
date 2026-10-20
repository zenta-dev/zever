package main

import (
	"context"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zenta-dev/zever/config"
)

// addUsageBody is the long description for `zever add -h`.
var addUsageBody = `Adds one battery to the calling project, or writes an opt-in recipe file.

Batteries (default instance):

  zever add <battery>[/<adapter>] [--adapter NAME] [--name N] [--uses R]

go/ast-edits internal/app/wiring.go: a Ref var, a container.Provide entry
and the adapter import, inserted idempotently without touching your code.
Appends the battery's keys to .env.example, adds the core+adapter requires
to go.mod (plus local replaces inside a framework checkout), then runs
` + "`go get`" + ` and ` + "`go mod tidy`" + `.

The adapter may be omitted (` + "`zever add cache`" + ` selects
config.Default()'s own pick, memory) or pinned explicitly
(` + "`zever add cache/redis`" + ` or ` + "`zever add cache --adapter redis`" + `).
When both the slash form and --adapter are given they must agree, or the
command fails with a conflict error.

--name wires a named instance (` + "`zever add db --name analytics`" + ` declares
` + "`var DBAnalytics = db.Ref(\"analytics\")`" + ` and a ` + "`DB__ANALYTICS__*`" + ` section in
.env.example). --uses declares a boot-order edge on another wired instance
(` + "`zever add cache --uses db`" + ` records the dependency via container.Provide1;
the other instance must already be wired). Swapping the adapter of an
already-wired instance (sqlite to postgres) edits one line plus the import
and drops the old import when nothing else uses it; it needs --force.

The unified diff of wiring.go always prints BEFORE anything is written.
--diff prints the diff and changes nothing; --yes applies without asking.
Without either, the command prompts (TTY required: otherwise re-run with
--yes). Re-adding the same battery+adapter is a no-op success.

Recipes (opt-in files, written from templates):

  zever add docker|ci|lint|compose|worker|seed|k8s

docker writes Dockerfile+.dockerignore, ci the GitHub workflow, lint the
golangci-lint config, compose compose.yaml (from the wired selections),
worker cmd/worker/main.go, seed db/seed/main.go, k8s k8s/app.yaml.
Existing files need --force; nothing else in your project is touched.

WHAT THIS DOES NOT DO: it never rewrites your code (wiring.go edits are
insert-only; a swap replaces exactly one entry), and it never touches
zever.yaml (config is env-only: see .env.example). Run it from your
project root (the directory holding go.mod).`

// addShortUsage returns the compact usage block an argument-count error
// carries: the command shape, the recipe names, and the pointer to the
// full reference. It replaces the 40-line addUsageBody in the error path,
// so a mistyped invocation prints three lines instead of a wall of prose.
func addShortUsage() string {
	return joinLines(
		"usage: zever add <battery>[/<adapter>] [--adapter NAME] [--name N] [--uses R]",
		"recipes: "+strings.Join(sortedKeys(addRecipeNames), ", "),
		formatHint("run 'zever add -h' for the full reference, 'zever new --list-batteries' for the battery list"),
	)
}

// addRecipeNames are the opt-in file recipes `zever add` writes from
// templates. No battery shares these names, so the dispatch is exact.
var addRecipeNames = map[string]bool{
	"docker": true, "ci": true, "lint": true, "compose": true,
	"worker": true, "seed": true, "k8s": true,
}

// DefaultGetTimeout bounds `go get` after wiring a battery.
const DefaultGetTimeout = 2 * time.Minute

// runGoGetFunc is a seam so tests stub the toolchain.
var runGoGetFunc = runGoGet

// newAddFlagSet builds the stdlib flag set for `zever add`: the single
// source for runAdd parsing and for help printers, so help can never
// drift from the real flags (mirrors newNewFlagSet).
func newAddFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.String("adapter", "", "adapter pick, overriding the positional battery/adapter slash form when both agree")
	fs.String("name", "", "wire a named instance (e.g. --name analytics for db.Ref(\"analytics\")) instead of the default")
	fs.String("uses", "", "declare a boot-order edge on another wired instance (battery or battery/name, e.g. --uses db)")
	fs.Bool("force", false, "allow adapter swaps and overwrite existing recipe files")
	fs.Bool("yes", false, "apply wiring.go changes without prompting")
	fs.Bool("y", false, "alias for --yes")
	fs.Bool("diff", false, "print the wiring.go diff and change nothing")
	fs.Bool("dry-run", false, "print the file edits without writing them")

	fs.Usage = func() {
		_, _ = fmt.Fprintln(fs.Output(), addUsageBody)
		printFlagDefaults(fs)
	}

	return fs
}

// addTarget is one resolved `zever add` battery request: the battery and
// effective adapter, plus the instance name ("" for the default) and the
// optional dependency edge (--uses, as battery or battery/name).
type addTarget struct {
	battery string
	adapter string
	name    string
	uses    string
}

// parseAddTarget splits a `zever add` positional into its battery and
// effective adapter, honouring --adapter, --name and --uses. A slash
// adapter and a different --adapter value is a conflict error. Known
// batteries resolve against allServiceAdapters (bare names take
// config.Default()'s own pick, and unknown adapters fail closed with
// choices); unknown names fail with a did-you-mean hint. Adapters with no
// standalone provider factory (session/cookie and friends) fail here, so
// no file is touched before the error.
func parseAddTarget(arg, flagAdapter, name, uses string) (addTarget, error) {
	const tag = "zever add"

	var tgt addTarget

	if arg == "" {
		return tgt, fmt.Errorf("%s: expected <battery>[/<adapter>] or a recipe (docker|ci|lint|compose|worker|seed|k8s), got an empty argument", tag)
	}

	if strings.Count(arg, "/") > 1 {
		return tgt, fmt.Errorf("%s: %q wants <battery>[/<adapter>] (a single optional /)", tag, arg)
	}

	battery, slashAdapter, _ := strings.Cut(arg, "/")

	if battery == "" || (strings.Contains(arg, "/") && slashAdapter == "") {
		return tgt, fmt.Errorf("%s: %q wants <battery>[/<adapter>] with no empty part", tag, arg)
	}

	if _, ok := allServiceAdapters[battery]; !ok {
		return tgt, unknownBatteryError(tag, battery)
	}

	if flagAdapter != "" && slashAdapter != "" && flagAdapter != slashAdapter {
		return tgt, fmt.Errorf("%s: adapter conflict: positional %q pins %q but --adapter selects %q (drop one or make them agree)", tag, arg, slashAdapter, flagAdapter)
	}

	adapter := slashAdapter
	if flagAdapter != "" {
		adapter = flagAdapter
	}

	if adapter == "" {
		adapter = defaultServiceAdapter(battery)
	}

	if choices := adaptersForBattery(battery); !batteryPicked(choices, adapter) {
		return tgt, unknownAdapterError(tag, battery, adapter, choices)
	}

	if name != "" {
		if strings.Contains(name, "__") {
			return tgt, fmt.Errorf("%s: invalid instance name %q: must not contain %q (it separates service, name and field in env keys)", tag, name, "__")
		}

		if !isValidAppName(name) {
			return tgt, fmt.Errorf("%s: invalid instance name %q (letters, digits, - and _ only)", tag, name)
		}
	}

	if uses != "" {
		useBattery, useName, _ := strings.Cut(uses, "/")

		if useBattery == "" || strings.Count(uses, "/") > 1 {
			return tgt, fmt.Errorf("%s: --uses %q wants <battery>[/<name>] with no empty part", tag, uses)
		}

		if _, ok := allServiceAdapters[useBattery]; !ok {
			return tgt, unknownBatteryError(tag, useBattery)
		}

		if useName != "" && !isValidAppName(useName) {
			return tgt, fmt.Errorf("%s: invalid --uses instance name %q (letters, digits, - and _ only)", tag, useName)
		}
	}

	tgt = addTarget{battery: battery, adapter: adapter, name: name, uses: uses}

	if _, err := factoryExpr(batterySelection{Battery: battery, Adapter: adapter}, "alias", "\t\t"); err != nil {
		return addTarget{}, err
	}

	return tgt, nil
}

// goModRequireVersion scans go.mod source for the version pinned by the
// require of modulePath, handling both single-line requires and require
// blocks. It returns "" when no require of that path exists.
func goModRequireVersion(src, modulePath string) string {
	inBlock := false

	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "require ("):
			inBlock = true

			continue
		case inBlock && trimmed == ")":
			inBlock = false

			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}

		if !inBlock {
			if len(fields) < 3 || fields[0] != "require" {
				continue
			}

			fields = fields[1:]
		}

		if fields[0] == modulePath {
			return strings.Trim(fields[1], `"`)
		}
	}

	return ""
}

// goModHasRequire reports whether go.mod source already requires modulePath
// (any version, either require form).
func goModHasRequire(src, modulePath string) bool {
	return goModRequireVersion(src, modulePath) != ""
}

// goModRootReplaceTarget scans go.mod source for a single-line
// `replace github.com/zenta-dev/zever => <target>` and returns the target
// (unquoted), or "" when there is none. Only the framework root replace
// is read: it is the marker of a local-checkout project, and the base
// every derived adapter replace is computed from.
func goModRootReplaceTarget(src string) string {
	for _, line := range strings.Split(src, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 4 && fields[0] == "replace" && fields[1] == frameworkModulePath && fields[2] == "=>" {
			return strings.Trim(fields[3], `"`)
		}
	}

	return ""
}

// isLocalReplaceTarget reports whether a replace target names a local
// directory (rather than a module version): derived adapter replaces are
// only emitted for those.
func isLocalReplaceTarget(target string) bool {
	return strings.HasPrefix(target, ".") || strings.HasPrefix(target, "/")
}

// pinnedFrameworkVersion returns the version the calling project pins its
// framework requires at: the first framework require found, or
// FrameworkVersion when the project requires none yet.
func pinnedFrameworkVersion(src string) string {
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)

		if len(fields) == 0 {
			continue
		}

		if fields[0] == "require" && len(fields) >= 3 {
			fields = fields[1:]
		} else if strings.HasPrefix(trimmed, "require (") {
			continue
		}

		if len(fields) >= 2 && strings.HasPrefix(fields[0], frameworkModulePath) {
			return strings.Trim(fields[1], `"`)
		}
	}

	return FrameworkVersion
}

// updateGoModForAdd ensures go.mod source requires the battery's core and
// adapter modules at version, adding local-checkout replaces derived from
// the framework root replace when the caller resolves against one. Paths
// already required (any version) or already replaced are left untouched.
func updateGoModForAdd(src string, sel batterySelection, version string) string {
	return ensureGoModRequires(src, []string{coreModulePath(sel.Battery), adapterModulePath(sel)}, version)
}

// ensureGoModRequires ensures go.mod source requires every listed module
// path at version, adding local-checkout replaces derived from the
// framework root replace when the caller resolves against one. Paths
// already required (any version) or already replaced are left untouched.
func ensureGoModRequires(src string, paths []string, version string) string {
	lines := strings.Split(src, "\n")

	// Insert new requires after the last single-line require, or inside
	// the first require block, or at the end when there are none.
	blockClose := -1
	lastSingle := -1

	inBlock := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "require ("):
			inBlock = true
		case inBlock && trimmed == ")":
			if blockClose == -1 {
				blockClose = i
			}

			inBlock = false
		case !inBlock && strings.HasPrefix(trimmed, "require "):
			lastSingle = i
		}
	}

	var missing []string

	for _, path := range paths {
		if !goModHasRequire(strings.Join(lines, "\n"), path) {
			missing = append(missing, path+" "+version)
		}
	}

	// Block entries are bare indented lines; single-line requires carry
	// the require keyword with no indent.
	for i, m := range missing {
		if blockClose != -1 {
			missing[i] = "\t" + m
		} else {
			missing[i] = "require " + m
		}
	}

	switch {
	case blockClose != -1:
		lines = append(lines[:blockClose], append(missing, lines[blockClose:]...)...)
	case lastSingle != -1:
		lines = append(lines[:lastSingle+1], append(missing, lines[lastSingle+1:]...)...)
	default:
		lines = append(lines, missing...)
	}

	out := strings.Join(lines, "\n")

	if target := goModRootReplaceTarget(out); isLocalReplaceTarget(target) {
		var extra []string

		for _, path := range paths {
			replace := "replace " + path + " => "
			if strings.Contains(out, replace) {
				continue
			}

			rel := filepath.ToSlash(filepath.Join(target, strings.TrimPrefix(path, frameworkModulePath+"/")))
			extra = append(extra, replace+rel)
		}

		if len(extra) > 0 {
			if !strings.HasSuffix(out, "\n") {
				out += "\n"
			}

			out += strings.Join(extra, "\n") + "\n"
		}
	}

	return out
}

// runGoGet runs `go get path@version` for each module path, so go.sum
// catches up before tidy. Paths already replaced to a local directory need
// no fetch and are skipped. A failure names the fix instead of raw go
// output.
func runGoGet(tag, dir string, paths []string, version string) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("%s: go toolchain not found on PATH, install Go (https://go.dev/doc/install) then run `go get` in %q manually: %w", tag, dir, err)
	}

	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return fmt.Errorf("%s: read go.mod: %w", tag, err)
	}

	rootTarget := goModRootReplaceTarget(string(gomod))

	var fetch []string

	for _, path := range paths {
		if rootTarget != "" && isLocalReplaceTarget(rootTarget) {
			replace := "replace " + path + " => "
			if strings.Contains(string(gomod), replace) {
				continue
			}
		}

		fetch = append(fetch, path+"@"+version)
	}

	for _, arg := range fetch {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultGetTimeout)

		cmd := exec.CommandContext(ctx, goBin, "get", arg)
		cmd.Dir = dir

		out, err := cmd.CombinedOutput()
		cancel()

		if err != nil {
			trimmed := strings.TrimSpace(string(out))
			if trimmed == "" {
				trimmed = err.Error()
			}

			return fmt.Errorf("%s: go get %s failed, run `go get %s` in %q manually: %w: %s", tag, arg, arg, dir, err, trimmed)
		}
	}

	return nil
}

// --- wiring.go editing (go/ast, insert-only) ---

// wiringImport is one import spec: Alias empty means a plain import.
type wiringImport struct {
	alias string
	path  string
}

// wiringProviderEntry is one parsed container.Provide entry: RefVar is the
// first argument (the Ref var name), DepVar the Provide1 dependency (empty
// for Provide), and start/end the element source range (excluding any
// trailing comma, which belongs to the literal, not the element).
type wiringProviderEntry struct {
	refVar   string
	depVar   string
	start    token.Pos
	end      token.Pos
	provides string
}

// wiringFile is the parsed shape of internal/app/wiring.go: the import
// block, the Providers slice literal, and every top-level Ref var.
type wiringFile struct {
	src      string
	fset     *token.FileSet
	imports  []wiringImport
	impDecl  token.Pos
	impStart token.Pos
	impEnd   token.Pos
	lit      *ast.CompositeLit
	entries  []wiringProviderEntry
	refVars  map[string]wiringRefDecl
	refOrder []string
	lastRef  token.Pos
}

// wiringRefDecl is one parsed Ref var: Pkg is the qualifier
// (db in db.DefaultRef()), Name the instance name ("default" for
// DefaultRef, the string otherwise).
type wiringRefDecl struct {
	pkg  string
	name string
	end  token.Pos
}

// parseWiringFile parses wiring.go source into its editable shape: import
// specs, the Providers slice literal with one entry per Provide call, and
// every `var X = <pkg>.Ref(...)/DefaultRef()` declaration. Anything else
// (user code) is opaque text the edit preserves byte-for-byte.
func parseWiringFile(src string) (*wiringFile, error) {
	const tag = "zever add"

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "wiring.go", src, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: parse internal/app/wiring.go: %w", tag, err)
	}

	wf := &wiringFile{src: src, fset: fset, refVars: map[string]wiringRefDecl{}}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}

		if gen.Tok == token.IMPORT {
			if err := parseWiringImports(wf, gen, tag); err != nil {
				return nil, err
			}

			continue
		}

		if gen.Tok == token.VAR {
			parseWiringRefVars(wf, gen)
		}
	}

	fn := funcDecl(file, "Providers")
	if fn == nil {
		return nil, fmt.Errorf("%s: internal/app/wiring.go has no Providers to extend (scaffold it with `zever new` first)", tag)
	}

	lit := providersLit(fn)
	if lit == nil {
		return nil, fmt.Errorf("%s: internal/app/wiring.go Providers does not return a []container.Provider literal", tag)
	}

	wf.lit = lit

	for _, elt := range lit.Elts {
		call, ok := elt.(*ast.CallExpr)
		if !ok {
			continue
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		provides := providesName(sel.Sel.Name)
		if provides == "" {
			continue
		}

		if len(call.Args) < 2 {
			continue
		}

		ref, ok := call.Args[0].(*ast.Ident)
		if !ok {
			continue
		}

		entry := wiringProviderEntry{refVar: ref.Name, start: elt.Pos(), end: elt.End(), provides: provides}

		if provides == "Provide1" && len(call.Args) >= 3 {
			if dep, ok := call.Args[1].(*ast.Ident); ok {
				entry.depVar = dep.Name
			}
		}

		wf.entries = append(wf.entries, entry)
	}

	return wf, nil
}

// parseWiringImports records the import block specs of gen into wf.
func parseWiringImports(wf *wiringFile, gen *ast.GenDecl, tag string) error {
	wf.impDecl, wf.impStart, wf.impEnd = gen.Pos(), gen.Lparen, gen.Rparen

	for _, spec := range gen.Specs {
		imp, ok := spec.(*ast.ImportSpec)
		if !ok {
			continue
		}

		unquoted, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return fmt.Errorf("%s: parse internal/app/wiring.go import: %w", tag, err)
		}

		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}

		wf.imports = append(wf.imports, wiringImport{alias: alias, path: unquoted})
	}

	return nil
}

// parseWiringRefVars records every `var X = <pkg>.Ref(...)/DefaultRef()`
// declaration of gen into wf.
func parseWiringRefVars(wf *wiringFile, gen *ast.GenDecl) {
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
			continue
		}

		call, ok := vs.Values[0].(*ast.CallExpr)
		if !ok {
			continue
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Ref" && sel.Sel.Name != "DefaultRef") {
			continue
		}

		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}

		name := "default"

		if sel.Sel.Name == "Ref" {
			if len(call.Args) != 1 {
				continue
			}

			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}

			unquoted, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}

			name = unquoted
		}

		wf.refVars[vs.Names[0].Name] = wiringRefDecl{pkg: pkg.Name, name: name, end: gen.End()}
		wf.refOrder = append(wf.refOrder, vs.Names[0].Name)

		if !wf.lastRef.IsValid() || gen.End() > wf.lastRef {
			wf.lastRef = gen.End()
		}
	}
}

// funcDecl returns the named function declaration, or nil.
func funcDecl(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name && fn.Recv == nil {
			return fn
		}
	}

	return nil
}

// providersLit returns the []container.Provider composite literal returned
// by fn, or nil when Providers builds the slice any other way (hand-edited
// beyond recognition: add refuses rather than guessing).
func providersLit(fn *ast.FuncDecl) *ast.CompositeLit {
	for _, stmt := range fn.Body.List {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}

		if lit, ok := ret.Results[0].(*ast.CompositeLit); ok {
			return lit
		}
	}

	return nil
}

// providesName maps a container.Provide* selector to its edge kind, or ""
// for anything else in the Providers literal (user helpers stay opaque).
func providesName(sel string) string {
	switch sel {
	case "Provide", "Provide1":
		return sel
	default:
		return ""
	}
}

// offset converts a token.Pos to a byte offset into wf.src.
func (wf *wiringFile) offset(pos token.Pos) int {
	return wf.fset.Position(pos).Offset
}

// importAliasFor returns the alias wiring.go already imports path under, or
// "" when the path is not imported.
func (wf *wiringFile) importAliasFor(path string) (string, bool) {
	for _, imp := range wf.imports {
		if imp.path == path {
			return imp.alias, true
		}
	}

	return "", false
}

// adapterOfAlias recovers the adapter pick from an entry's factory alias:
// the generated alias is battery+adapter (dbsqlite), so stripping the
// battery prefix and validating against the battery's adapters inverts it.
// Hand-renamed aliases fall back to the import path's directory.
func adapterOfAlias(battery, alias, path string) string {
	if strings.HasPrefix(alias, battery) {
		if cand := strings.TrimPrefix(alias, battery); batteryPicked(adaptersForBattery(battery), cand) {
			return cand
		}
	}

	rest, ok := strings.CutPrefix(path, frameworkModulePath+"/adapters/")
	if !ok {
		return ""
	}

	_, dir, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}

	switch {
	case battery == "password" && dir == "argon2":
		return "argon2id"
	case battery == "resilience" && dir == "inproc":
		return "memory"
	default:
		return dir
	}
}

// entryFor returns the provider entry wiring refVar, or nil.
func (wf *wiringFile) entryFor(refVar string) *wiringProviderEntry {
	for i := range wf.entries {
		if wf.entries[i].refVar == refVar {
			return &wf.entries[i]
		}
	}

	return nil
}

// renderImportBlock re-emits the import block from specs sorted by path,
// one spec per line. Only the import block normalizes; every other byte of
// the file is preserved.
func renderImportBlock(specs []wiringImport) string {
	sort.Slice(specs, func(i, j int) bool { return specs[i].path < specs[j].path })

	var b strings.Builder

	b.WriteString("import (\n")

	for _, imp := range specs {
		if imp.alias == "" {
			fmt.Fprintf(&b, "\t%q\n", imp.path)
		} else {
			fmt.Fprintf(&b, "\t%s %q\n", imp.alias, imp.path)
		}
	}

	b.WriteString(")")

	return b.String()
}

// editWiring computes the new wiring.go source for tgt: inserting the Ref
// var, the Provide entry and the imports, or swapping the entry+import for
// a --force adapter change. It returns the new source, a changed flag, and
// an error for anything it refuses (missing dep, swap without --force).
// The edit is insert-only except for a forced swap, which replaces exactly
// the old entry and its now-unused import.
func editWiring(wf *wiringFile, tgt addTarget, force bool) (string, bool, error) {
	const tag = "zever add"

	sel := batterySelection{Battery: tgt.battery, Adapter: tgt.adapter}
	varName := refVarName(tgt.battery, tgt.name)

	instName := tgt.name
	if instName == "" {
		instName = "default"
	}

	// Resolve the dep edge first: nothing is edited when --uses names an
	// instance that is not wired yet.
	depVar := ""
	if tgt.uses != "" {
		useBattery, useName, _ := strings.Cut(tgt.uses, "/")
		if useName == "" {
			useName = "default"
		}

		found := ""
		for name, decl := range wf.refVars {
			if decl.pkg == useBattery && decl.name == useName {
				found = name
				break
			}
		}

		if found == "" {
			want := refVarName(useBattery, useName)
			return "", false, fmt.Errorf("%s: --uses %q is not wired yet (want Ref var %s): run `zever add %s%s` first",
				tag, tgt.uses, want, useBattery, useNameSuffix(useName))
		}

		depVar = found
	}

	factory, err := factoryExpr(sel, adapterBindingAlias(sel), "\t\t")
	if err != nil {
		return "", false, err
	}

	entry := "container.Provide(" + varName + ", " + factory + ")"
	if depVar != "" {
		if _, ok := coreServiceTypes[tgt.battery]; !ok {
			return "", false, fmt.Errorf("%s: battery %q has no --uses factory shape", tag, tgt.battery)
		}

		depPkg := usesDepPkg(wf, depVar)
		if depPkgIface(depPkg) == "Any" {
			return "", false, fmt.Errorf("%s: --uses %q: cannot resolve the dependency type of %s", tag, tgt.uses, depVar)
		}

		entry = usesProvideEntry(tgt.battery, varName, depVar, depPkg, factory)
	}

	existing := wf.entryFor(varName)
	src := wf.src

	if existing != nil {
		cur := adapterOfEntry(wf, existing)
		if cur == tgt.adapter {
			return src, false, nil
		}

		if !force {
			return "", false, fmt.Errorf("%s: %q is already wired with adapter %q (pass --force to swap to %q)", tag, varName, cur, tgt.adapter)
		}

		// Forced swap: replace exactly the old entry and refresh the
		// generated Ref doc, then drop the old adapter import when
		// nothing else references it.
		start, end := wf.offset(existing.start), wf.offset(existing.end)
		src = src[:start] + entry + src[end:]
		src = refreshRefDoc(src, varName, tgt.battery, instName, tgt.adapter)

		swapped, parseErr := parseWiringFile(src)
		if parseErr != nil {
			return "", false, parseErr
		}

		src = swapAdapterImport(swapped, sel, cur)

		if validErr := validateWiringGo(src); validErr != nil {
			return "", false, validErr
		}

		return src, true, nil
	}

	// Fresh instance: insert the Ref var after the last one, the entry
	// before the literal's closing brace, and any missing imports.
	if !wf.lastRef.IsValid() {
		return "", false, fmt.Errorf("%s: internal/app/wiring.go has no Ref var to extend (scaffold it with `zever new` first)", tag)
	}

	if !wf.impDecl.IsValid() || !wf.impStart.IsValid() || !wf.impEnd.IsValid() {
		return "", false, fmt.Errorf("%s: internal/app/wiring.go has no import block to extend (scaffold it with `zever new` first)", tag)
	}

	refDecl := "var " + varName + " = " + tgt.battery + "." + refCall(instName)
	refDoc := "// " + varName + " names "

	if instName == "default" {
		refDoc += "the default " + tgt.battery + " instance (" + tgt.adapter + " adapter)."
	} else {
		refDoc += "the " + tgt.battery + "/" + instName + " instance (" + tgt.adapter + " adapter)."
	}

	if _, ok := wf.refVars[varName]; !ok {
		at := wf.offset(wf.lastRef)
		src = src[:at] + "\n\n" + refDoc + "\n" + refDecl + src[at:]
	}

	refreshed, err := parseWiringFile(src)
	if err != nil {
		return "", false, err
	}

	rbrace := refreshed.offset(refreshed.lit.Rbrace)
	lineStart := strings.LastIndex(src[:rbrace], "\n") + 1
	indent := src[lineStart:rbrace]
	src = src[:rbrace] + "\t" + entry + ",\n" + indent + src[rbrace:]

	src = ensureWiringImports(src, refreshed, sel, adapterBindingAlias(sel))

	if validErr := validateWiringGo(src); validErr != nil {
		return "", false, validErr
	}

	return src, true, nil
}

// validateWiringGo parses src, failing with an actionable error when the
// edit produced invalid Go instead of raw parser output.
func validateWiringGo(src string) error {
	const tag = "zever add"

	if _, err := parser.ParseFile(token.NewFileSet(), "wiring.go", src, 0); err != nil {
		return fmt.Errorf("%s: updated internal/app/wiring.go is not valid Go: %w", tag, err)
	}

	return nil
}

// useNameSuffix renders the --name flag suffix for a hint command.
func useNameSuffix(name string) string {
	if name == "" || name == "default" {
		return ""
	}

	return " --name " + name
}

// refreshRefDoc rewrites the generated `// VAR names ...` doc line above
// `var VAR = ...` after an adapter swap. Hand-written docs (anything not
// starting with the generated prefix) are left untouched.
func refreshRefDoc(src, varName, battery, instName, adapter string) string {
	doc := "// " + varName + " names "
	if instName == "default" {
		doc += "the default " + battery + " instance (" + adapter + " adapter)."
	} else {
		doc += "the " + battery + "/" + instName + " instance (" + adapter + " adapter)."
	}

	lines := strings.Split(src, "\n")

	for i, line := range lines {
		if strings.HasPrefix(line, "var "+varName+" = ") && i > 0 {
			if strings.HasPrefix(lines[i-1], "// "+varName+" names ") {
				lines[i-1] = doc
			}

			break
		}
	}

	return strings.Join(lines, "\n")
}

// refCall renders the Ref constructor call for an instance name.
func refCall(name string) string {
	if name == "" || name == "default" {
		return "DefaultRef()"
	}

	return "Ref(" + strconv.Quote(name) + ")"
}

// adapterBindingAlias returns the adapter import alias for sel (see
// adapterBinding, which also returns the path and Register call).
func adapterBindingAlias(sel batterySelection) string {
	alias, _ := adapterBinding(sel)

	return alias
}

// adapterOfEntry recovers the wired adapter of an existing provider entry
// from its factory alias (or import path fallback).
func adapterOfEntry(wf *wiringFile, entry *wiringProviderEntry) string {
	elt := wf.src[wf.offset(entry.start):wf.offset(entry.end)]

	for _, imp := range wf.imports {
		if imp.alias != "" && strings.Contains(elt, imp.alias+".") {
			if decl, ok := wf.refVars[entry.refVar]; ok {
				return adapterOfAlias(decl.pkg, imp.alias, imp.path)
			}
		}
	}

	return ""
}

// usesDepPkg resolves the core package qualifier of the --uses dependency
// var from its Ref declaration (db in `var DB = db.DefaultRef()`). The
// caller verified the var exists; "" means hand-edited beyond recognition.
func usesDepPkg(wf *wiringFile, depVar string) string {
	if decl, ok := wf.refVars[depVar]; ok {
		return decl.pkg
	}

	return ""
}

// usesProvideEntry renders a Provide1 entry wiring newVar against depVar:
// the factory keeps the battery's own constructor and ignores the resolved
// dependency value, recording a boot-order and graph-validation edge.
func usesProvideEntry(battery, newVar, depVar, depPkg, factory string) string {
	types, ok := coreServiceTypes[battery]
	if !ok {
		types = [2]string{"Any", "Options"}
	}

	depIface := depPkgIface(depPkg)

	return fmt.Sprintf("container.Provide1(%s, %s, func(o %s.Options, _ %s.%s) (%s.%s, error) {\n\t\t\treturn %s\n\t\t})",
		newVar, depVar, battery, depPkg, depIface, battery, types[0], bareFactoryCall(factory))
}

// depPkgIface maps a core package qualifier to its service interface type,
// mirroring each core battery's Ref helper.
func depPkgIface(pkg string) string {
	if iface, ok := map[string]string{
		"ai": "AI", "analytics": "Analytics", "auth": "Auth", "billing": "Billing",
		"cache": "Cache", "cdn": "CDN", "crypto": "Crypto", "db": "DB",
		"document": "Document", "eventbus": "EventBus", "flag": "Flag", "geo": "Geo",
		"i18n": "I18n", "idempotency": "Store", "lock": "Locker", "log": "Logger",
		"mailer": "Mailer", "media": "Media", "notification": "Notifier",
		"observability": "Provider", "outbox": "Store", "password": "Hasher",
		"payment": "Payment", "permission": "Checker", "queue": "Queue",
		"ratelimit": "Limiter", "resilience": "Manager", "router": "Router",
		"scheduler": "Scheduler", "search": "Search", "secrets": "Secrets",
		"session": "Store", "storage": "Storage", "tenant": "Tenant",
		"vectorstore": "VectorStore", "webhook": "Webhook", "workflow": "Workflow",
	}[pkg]; ok {
		return iface
	}

	return "Any"
}

// bareFactoryCall unwraps a factory expression into the constructor call
// body for a --uses closure: WrapNoErr/WrapNoOpts(X.New) becomes X.New(o)
// or X.New(), and a func literal is returned as-is (already complete).
func bareFactoryCall(factory string) string {
	if rest, ok := strings.CutPrefix(factory, "container.WrapNoErr("); ok {
		return strings.TrimSuffix(rest, ")") + "(o)"
	}

	if rest, ok := strings.CutPrefix(factory, "container.WrapNoOpts("); ok {
		return strings.TrimSuffix(rest, ")") + "()"
	}

	if strings.HasPrefix(factory, "func(") {
		inner := factory
		if i := strings.Index(inner, "return "); i != -1 {
			if j := strings.LastIndex(inner, "\n\t}"); j != -1 {
				return strings.TrimSpace(inner[i+len("return ") : j])
			}
		}

		return factory
	}

	return factory + "(o)"
}

// ensureWiringImports inserts the adapter and core imports for sel when
// missing, reusing the existing alias when the path is already imported.
// Only the import block normalizes; user code is untouched.
func ensureWiringImports(src string, wf *wiringFile, sel batterySelection, alias string) string {
	_, path := adapterBinding(sel)
	core := coreModulePath(sel.Battery)

	specs := append([]wiringImport(nil), wf.imports...)

	if _, ok := wf.importAliasFor(path); !ok {
		specs = append(specs, wiringImport{alias: alias, path: path})
	}

	foundCore := false

	for _, imp := range specs {
		if imp.path == core {
			foundCore = true
		}
	}

	if !foundCore {
		specs = append(specs, wiringImport{path: core})
	}

	start, end := wf.offset(wf.impDecl), wf.offset(wf.impEnd)+len(")")

	return src[:start] + renderImportBlock(specs) + src[end:]
}

// swapAdapterImport replaces the old adapter import with the new one when
// nothing else references the old alias, and ensures the core and new
// adapter imports exist. Only the import block normalizes.
func swapAdapterImport(wf *wiringFile, sel batterySelection, curAdapter string) string {
	_, newPath := adapterBinding(sel)
	core := coreModulePath(sel.Battery)

	oldSel := batterySelection{Battery: sel.Battery, Adapter: curAdapter}
	_, oldPath := adapterBinding(oldSel)

	oldAlias, _ := wf.importAliasFor(oldPath)
	uses := 0

	if oldAlias != "" {
		needle := oldAlias + "."
		for _, entry := range wf.entries {
			elt := wf.src[wf.offset(entry.start):wf.offset(entry.end)]
			uses += strings.Count(elt, needle)
		}
	}

	specs := make([]wiringImport, 0, len(wf.imports))

	for _, imp := range wf.imports {
		if imp.path == oldPath && uses == 0 {
			continue
		}

		specs = append(specs, imp)
	}

	if _, ok := wf.importAliasFor(newPath); !ok {
		alias, _ := adapterBinding(sel)
		specs = append(specs, wiringImport{alias: alias, path: newPath})
	}

	foundCore := false

	for _, imp := range specs {
		if imp.path == core {
			foundCore = true
		}
	}

	if !foundCore {
		specs = append(specs, wiringImport{path: core})
	}

	start, end := wf.offset(wf.impDecl), wf.offset(wf.impEnd)+len(")")

	return wf.src[:start] + renderImportBlock(specs) + wf.src[end:]
}

// unifiedDiff renders before→after as a unified diff with context lines of
// context, in `--- a/path` / `+++ b/path` form. Pure insertions, pure
// deletions and replacements all render; identical inputs return "".
func unifiedDiff(path, before, after string, context int) string {
	a := strings.Split(before, "\n")
	b := strings.Split(after, "\n")

	ops := diffOps(a, b)

	// Line numbers after each op: walk once, recording (op, ai, bi) where
	// ai/bi are the a/b line indices the op consumes (-1 when none).
	type aligned struct {
		op     byte
		ai, bi int
	}

	var al []aligned

	ai, bi := 0, 0

	for _, op := range ops {
		switch op.op {
		case ' ':
			al = append(al, aligned{' ', ai, bi})
			ai++
			bi++
		case '-':
			al = append(al, aligned{'-', ai, -1})
			ai++
		case '+':
			al = append(al, aligned{'+', -1, bi})
			bi++
		}
	}

	// Hunk windows: every change pulls in context lines on each side;
	// overlapping windows merge.
	type window struct{ lo, hi int }

	var windows []window

	for i, e := range al {
		if e.op == ' ' {
			continue
		}

		lo, hi := i-context, i+context+1
		if lo < 0 {
			lo = 0
		}

		if hi > len(al) {
			hi = len(al)
		}

		if n := len(windows); n > 0 && lo <= windows[n-1].hi {
			windows[n-1].hi = hi
		} else {
			windows = append(windows, window{lo, hi})
		}
	}

	if len(windows) == 0 {
		return ""
	}

	var out strings.Builder

	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", path, path)

	for _, w := range windows {
		aStart, bStart := -1, -1
		aLen, bLen := 0, 0

		for i := w.lo; i < w.hi; i++ {
			e := al[i]
			if e.ai >= 0 {
				if aStart == -1 {
					aStart = e.ai
				}

				aLen++
			}

			if e.bi >= 0 {
				if bStart == -1 {
					bStart = e.bi
				}

				bLen++
			}
		}

		if aStart == -1 {
			aStart = len(a)
		}

		if bStart == -1 {
			bStart = len(b)
		}

		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", aStart+1, aLen, bStart+1, bLen)

		for i := w.lo; i < w.hi; i++ {
			e := al[i]

			switch e.op {
			case ' ':
				out.WriteString(" " + a[e.ai] + "\n")
			case '-':
				out.WriteString("-" + a[e.ai] + "\n")
			case '+':
				out.WriteString("+" + b[e.bi] + "\n")
			}
		}
	}

	return out.String()
}

// diffOp is one LCS backtrack step.
type diffOp struct {
	op byte
}

// diffOps aligns a and b with an LCS table, returning the edit script as
// ' ' (equal), '-' (in a only) and '+' (in b only) ops.
func diffOps(a, b []string) []diffOp {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)

	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}

	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var ops []diffOp

	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' '})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{'-'})
			i++
		default:
			ops = append(ops, diffOp{'+'})
			j++
		}
	}

	for ; i < n; i++ {
		ops = append(ops, diffOp{'-'})
	}

	for ; j < m; j++ {
		ops = append(ops, diffOp{'+'})
	}

	return ops
}

// --- .env.example editing ---

// envExampleSection renders one .env.example section for sel: the adapter
// line plus every key and default from the battery's Options structs,
// sharing renderEnvExample's generator so `new` and `add` cannot drift.
func envExampleSection(sel batterySelection, name string) string {
	var b strings.Builder

	prefix := strings.ToUpper(sel.Battery)
	sep := "_"
	header := fmt.Sprintf("# --- %s (adapter %s) ---\n", sel.Battery, sel.Adapter)

	if name != "" && name != "default" {
		prefix += "__" + strings.ToUpper(name)
		sep = "__"
		header = fmt.Sprintf("# --- %s (adapter %s, instance %s) ---\n", sel.Battery, sel.Adapter, name)
	}

	b.WriteString(header)
	fmt.Fprintf(&b, "%s%sADAPTER=%s\n", prefix, sep, sel.Adapter)

	if opts := config.DefaultOptions(sel.Battery, name); opts != nil {
		appendEnvKeys(&b, prefix, sep, opts)
	}

	return b.String()
}

// appendEnvExample inserts section into the .env.example source when no
// section for the same battery+instance exists yet, returning the updated
// source and whether it changed. Idempotent: re-adding is a no-op.
func appendEnvExample(src, section, battery, name string) (string, bool) {
	if hasEnvSection(src, battery, name) {
		return src, false
	}

	if src != "" && !strings.HasSuffix(src, "\n") {
		src += "\n"
	}

	if src != "" {
		src += "\n"
	}

	return src + section, true
}

// hasEnvSection reports whether src already holds the section for
// battery+instance.
func hasEnvSection(src, battery, name string) bool {
	if name != "" && name != "default" {
		needle := "# --- " + battery + " (adapter"
		suffix := ", instance " + name + ") ---"

		for _, line := range strings.Split(src, "\n") {
			if strings.HasPrefix(line, needle) && strings.HasSuffix(line, suffix) {
				return true
			}
		}

		return false
	}

	needle := "# --- " + battery + " (adapter"

	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, needle) && !strings.Contains(line, ", instance ") {
			return true
		}
	}

	return false
}

// --- runAdd ---

// runAdd implements `zever add <battery>[/<adapter>] [--adapter] [--name]
// [--uses] [--force] [--yes] [--diff] [--dry-run]` plus the opt-in recipes
// `zever add docker|ci|lint|compose|worker|seed|k8s`: one positional, no
// prompting except the wiring.go apply confirm (skipped by --yes/--diff).
func runAdd(args []string) error {
	const tag = "zever add"

	fs := newAddFlagSet()

	positional, err := flexibleParse(fs, args)
	if err != nil {
		return err
	}

	flagString := func(name string) string {
		if f := fs.Lookup(name); f != nil {
			return f.Value.String()
		}

		return ""
	}

	if len(positional) != 1 {
		return fmt.Errorf("%s: expected exactly one <battery>[/<adapter>] or recipe, got %d\n%s", tag, len(positional), addShortUsage())
	}

	force := flagBool(fs, "force")
	dryRun := flagBool(fs, "dry-run")
	diffOnly := flagBool(fs, "diff")
	yes := flagBool(fs, "yes") || flagBool(fs, "y")

	if addRecipeNames[positional[0]] {
		return runAddRecipe(tag, positional[0], force, dryRun, diffOnly)
	}

	tgt, err := parseAddTarget(positional[0], flagString("adapter"), flagString("name"), flagString("uses"))
	if err != nil {
		return err
	}

	if dryRun {
		_, _ = fmt.Fprintln(os.Stdout, dim("dry run — no files written"))
	}

	return addBatteryToProject(tag, tgt, force, dryRun, diffOnly, yes)
}

// addBatteryToProject performs the `zever add` battery edits against the
// calling project in the working directory: the go/ast wiring.go edit, the
// .env.example append, the go.mod requires (+ local replaces), then `go
// get` and `go mod tidy`. The wiring.go unified diff prints before anything
// is written; --diff prints only the diff, --yes skips the confirm prompt.
func addBatteryToProject(tag string, tgt addTarget, force, dryRun, diffOnly, yes bool) error {
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no go.mod in the working directory (run this from your project root; scaffold one with `zever new`)", tag)
		}

		return fmt.Errorf("%s: read go.mod: %w", tag, err)
	}

	appPath := filepath.Join("internal", "app", "wiring.go")

	appSrc, err := os.ReadFile(appPath) // project-owned scaffold path
	if err != nil {
		return fmt.Errorf("%s: read %q: %w (scaffold it with `zever new` first)", tag, appPath, err)
	}

	wf, err := parseWiringFile(string(appSrc))
	if err != nil {
		return err
	}

	updated, changed, err := editWiring(wf, tgt, force)
	if err != nil {
		return err
	}

	sel := batterySelection{Battery: tgt.battery, Adapter: tgt.adapter}

	if !changed {
		_, _ = fmt.Fprintln(os.Stdout, dim("already wired ")+cyan(tgt.battery)+dim(" with adapter ")+cyan(tgt.adapter))

		return printAddNext(tgt)
	}

	diff := unifiedDiff(filepath.ToSlash(appPath), string(appSrc), updated, 3)
	_, _ = fmt.Fprintln(os.Stdout, diff)

	if diffOnly {
		return nil
	}

	if dryRun {
		_, _ = fmt.Fprintln(os.Stdout, dim("would write ")+cyan(appPath))
		_, _ = fmt.Fprintln(os.Stdout, dim("would append ")+cyan(".env.example"))
		_, _ = fmt.Fprintln(os.Stdout, dim("would update ")+cyan("go.mod"))

		return printAddNext(tgt)
	}

	if !yes {
		apply, promptErr := promptConfirm("Apply these wiring.go changes?", "Yes")
		if promptErr != nil {
			return fmt.Errorf("%s: %w (re-run with --yes to apply without prompting)", tag, promptErr)
		}

		if !apply {
			_, _ = fmt.Fprintln(os.Stdout, dim("aborted — wiring.go unchanged"))

			return nil
		}
	}

	if werr := os.WriteFile(appPath, []byte(updated), 0o644); werr != nil { //nolint:gosec // project-owned scaffold path
		return fmt.Errorf("%s: write %q: %w", tag, appPath, werr)
	}

	example, err := readOrEmpty(".env.example")
	if err != nil {
		return fmt.Errorf("%s: read .env.example: %w", tag, err)
	}

	section := envExampleSection(sel, tgt.name)

	updatedExample, _ := appendEnvExample(string(example), section, tgt.battery, tgt.name)
	if updatedExample != string(example) {
		if werr := os.WriteFile(".env.example", []byte(updatedExample), 0o644); werr != nil { //nolint:gosec // project config, not a secret
			return fmt.Errorf("%s: write .env.example: %w", tag, werr)
		}
	}

	version := pinnedFrameworkVersion(string(gomod))
	updatedGomod := updateGoModForAdd(string(gomod), sel, version)

	if updatedGomod != string(gomod) {
		if werr := os.WriteFile("go.mod", []byte(updatedGomod), 0o644); werr != nil { //nolint:gosec // go.mod is not a secret
			return fmt.Errorf("%s: write go.mod: %w", tag, werr)
		}
	}

	paths := []string{coreModulePath(sel.Battery), adapterModulePath(sel)}

	if gerr := runGoGetFunc(tag, ".", paths, version); gerr != nil {
		return gerr
	}

	if terr := runGoModTidyFunc(tag, "."); terr != nil {
		return terr
	}

	_, _ = fmt.Fprintln(os.Stdout, success("✔ added ")+bold(tgt.battery)+dim(" with adapter ")+cyan(tgt.adapter))

	return printAddNext(tgt)
}

// readOrEmpty reads path, returning empty bytes when it does not exist.
func readOrEmpty(path string) ([]byte, error) {
	data, err := os.ReadFile(path) // project-owned path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}

	return data, nil
}

// printAddNext prints the follow-up commands after a successful add.
func printAddNext(tgt addTarget) error {
	next := joinLines(
		bold("Next steps:"),
		"  "+dim("1.")+"  "+cmd("go mod tidy"),
		"  "+dim("2.")+"  "+cmd("zever serve"),
	)

	_, _ = fmt.Fprintln(os.Stdout, "")

	if colorEnabled {
		_, _ = fmt.Fprintln(os.Stdout, box(next))
	} else {
		_, _ = fmt.Fprintln(os.Stdout, next)
	}

	if shouldShowHint() {
		_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("battery %q is now wired in internal/app/wiring.go", tgt.battery)))
	}

	return nil
}

// runAddRecipe writes one opt-in recipe's files from templates: docker,
// ci, lint, compose, worker, seed or k8s. Existing files need --force;
// --dry-run and --diff print instead of writing.
func runAddRecipe(tag, recipe string, force, dryRun, diffOnly bool) error {
	if _, err := os.Stat("go.mod"); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no go.mod in the working directory (run this from your project root; scaffold one with `zever new`)", tag)
		}

		return fmt.Errorf("%s: stat go.mod: %w", tag, err)
	}

	files, err := recipeFiles(recipe)
	if err != nil {
		return err
	}

	if dryRun || diffOnly {
		_, _ = fmt.Fprintln(os.Stdout, dim("dry run — no files written"))

		for _, f := range files {
			_, _ = fmt.Fprintln(os.Stdout, dim("would write ")+cyan(f.rel))
		}

		return nil
	}

	wrote := []string{}

	for _, f := range files {
		if err := writeScaffold(tag, f.rel, []byte(f.content), force); err != nil {
			return fmt.Errorf("%s: recipe %q: %w (pass --force to overwrite %s)", tag, recipe, err, f.rel)
		}

		wrote = append(wrote, f.rel)
	}

	for _, path := range wrote {
		_, _ = fmt.Fprintln(os.Stdout, "  "+dim("wrote ")+cyan(path))
	}

	if recipe == "worker" || recipe == "seed" {
		if gerr := ensureRecipeGoMod(tag); gerr != nil {
			return gerr
		}
	}

	_, _ = fmt.Fprintln(os.Stdout, success("✔ added recipe ")+bold(recipe))

	return nil
}

// recipeFile is one file a recipe writes: rel path plus content.
type recipeFile struct {
	rel     string
	content string
}

// recipeFiles renders every file for a recipe from templates. The worker
// and seed mains only import already-required modules (serve, container),
// so no go.mod edit is needed for them; compose renders from the wired
// selections parsed out of internal/app/wiring.go.
func recipeFiles(recipe string) ([]recipeFile, error) {
	const tag = "zever add"

	modulePath := ""
	if data, err := os.ReadFile("go.mod"); err == nil {
		modulePath = modulePathOf("go.mod")
		_ = data
	}

	if modulePath == "" {
		modulePath = "app"
	}

	goVersion := goVersionOf("go.mod")
	if goVersion == "" {
		goVersion = defaultGoVersion
	}

	switch recipe {
	case "docker":
		return []recipeFile{
			{rel: "Dockerfile", content: renderNewDockerfile(goVersion)},
			{rel: ".dockerignore", content: newDockerignore},
		}, nil
	case "ci":
		pin := "latest"
		if data, err := os.ReadFile("go.mod"); err == nil {
			if v := pinnedFrameworkVersion(string(data)); v != "" && v != pseudoVersionZero {
				pin = v
			}
		}

		return []recipeFile{
			{rel: filepath.Join(".github", "workflows", "ci.yml"), content: renderCIGitHubWorkflow(goVersion, pin, "zenorm,proto,atlas")},
		}, nil
	case "lint":
		return []recipeFile{{rel: ".golangci.yml", content: newGolangciYml}}, nil
	case "compose":
		sel, err := wiringSelections()
		if err != nil {
			return nil, err
		}

		compose := renderNewCompose(sel)
		if compose == "" {
			return nil, fmt.Errorf("%s: recipe compose: every wired battery is local-only (sqlite/memory), nothing to compose; add a postgres or redis battery first", tag)
		}

		return []recipeFile{{rel: "compose.yaml", content: compose}}, nil
	case "worker":
		content, err := renderGoFile(tag, "worker main.go", recipeWorkerMainTemplate, struct{ ModulePath string }{modulePath})
		if err != nil {
			return nil, err
		}

		return []recipeFile{{rel: filepath.Join(defaultWorkerEntry, "main.go"), content: string(content)}}, nil
	case "seed":
		content, err := renderGoFile(tag, "seed main.go", recipeSeedMainTemplate, struct{ ModulePath string }{modulePath})
		if err != nil {
			return nil, err
		}

		return []recipeFile{{rel: filepath.Join(defaultSeedEntry, "main.go"), content: string(content)}}, nil
	case "k8s":
		appName := filepath.Base(modulePath)
		if appName == "" || appName == "." {
			appName = "app"
		}

		return []recipeFile{{rel: filepath.Join("k8s", "app.yaml"), content: renderK8sManifest(appName)}}, nil
	default:
		return nil, fmt.Errorf("%s: unknown recipe %q (choices: docker, ci, lint, compose, worker, seed, k8s)", tag, recipe)
	}
}

// wiringSelections parses the wired battery selections out of
// internal/app/wiring.go imports for recipes (compose) that render from
// the current wiring. Batteries resolve from adapter import paths; unknown
// shapes fail closed.
func wiringSelections() ([]batterySelection, error) {
	const tag = "zever add"

	data, err := os.ReadFile(filepath.Join("internal", "app", "wiring.go"))
	if err != nil {
		return nil, fmt.Errorf("%s: read internal/app/wiring.go: %w (scaffold it with `zever new` first)", tag, err)
	}

	wf, err := parseWiringFile(string(data))
	if err != nil {
		return nil, err
	}

	var sel []batterySelection

	for _, imp := range wf.imports {
		rest, ok := strings.CutPrefix(imp.path, frameworkModulePath+"/adapters/")
		if !ok {
			continue
		}

		battery, dir, ok := strings.Cut(rest, "/")
		if !ok {
			continue
		}

		adapter := dir
		switch {
		case battery == "password" && dir == "argon2":
			adapter = "argon2id"
		case battery == "resilience" && dir == "inproc":
			adapter = "memory"
		case dir == "db" && (battery == "search" || battery == "vectorstore" || battery == "workflow"):
			adapter = "db"
		}

		sel = append(sel, batterySelection{Battery: battery, Adapter: adapter})
	}

	sort.Slice(sel, func(i, j int) bool {
		if sel[i].Battery != sel[j].Battery {
			return sel[i].Battery < sel[j].Battery
		}

		return sel[i].Adapter < sel[j].Adapter
	})

	return sel, nil
}

// ensureRecipeGoMod ensures the worker/seed entrypoints' modules resolve:
// serve and container requires (plus local replaces) when missing.
func ensureRecipeGoMod(tag string) error {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("%s: read go.mod: %w", tag, err)
	}

	version := pinnedFrameworkVersion(string(data))
	updated := ensureGoModRequires(string(data),
		[]string{frameworkModulePath + "/serve", frameworkModulePath + "/container"}, version)

	if updated == string(data) {
		return nil
	}

	if werr := os.WriteFile("go.mod", []byte(updated), 0o644); werr != nil { //nolint:gosec // go.mod is not a secret
		return fmt.Errorf("%s: write go.mod: %w", tag, werr)
	}

	if terr := runGoModTidyFunc(tag, "."); terr != nil {
		return terr
	}

	return nil
}
