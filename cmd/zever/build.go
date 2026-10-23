package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// ErrBuildUnknownAdapter is returned when --adapters names a battery or
// battery/adapter pair the CLI does not know.
var ErrBuildUnknownAdapter = errors.New("zever build: unknown adapter")

// resolveBuildAdapters resolves a `zever build --adapters` value into the
// battery selections a slim CLI registers. Each comma-separated entry is a
// bare battery (selecting every known adapter for it) or a battery/adapter
// pin (selecting one adapter, legacy aliases included). Entries are
// trimmed, deduped, and sorted by battery then adapter.
//
// An empty value returns nil, meaning the full set: the slim build keeps
// every registered adapter, identical to the default CLI.
func resolveBuildAdapters(raw string) ([]batterySelection, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	seen := map[batterySelection]bool{}

	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}

		battery, adapter, _ := strings.Cut(tok, "/")

		if _, ok := allServiceAdapters[battery]; !ok {
			return nil, fmt.Errorf("%w %q (want one of: %s)", ErrBuildUnknownAdapter, tok, strings.Join(sortedKeys(allServiceAdapters), ", "))
		}

		if adapter == "" {
			for _, a := range adaptersForBattery(battery) {
				seen[batterySelection{Battery: battery, Adapter: a}] = true
			}

			continue
		}

		found := false

		for _, c := range adaptersForBattery(battery) {
			if c == adapter {
				found = true

				break
			}
		}

		if !found {
			return nil, fmt.Errorf("%w %q (want one of: %s)", ErrBuildUnknownAdapter, tok, strings.Join(adaptersForBattery(battery), ", "))
		}

		seen[batterySelection{Battery: battery, Adapter: adapter}] = true
	}

	if len(seen) == 0 {
		return nil, nil
	}

	sel := make([]batterySelection, 0, len(seen))
	for b := range seen {
		sel = append(sel, b)
	}

	sort.Slice(sel, func(i, j int) bool {
		if sel[i].Battery != sel[j].Battery {
			return sel[i].Battery < sel[j].Battery
		}

		return sel[i].Adapter < sel[j].Adapter
	})

	return sel, nil
}

// ErrBuildSource is returned when the cmd/zever source checkout cannot be
// found or read for a slim build.
var ErrBuildSource = errors.New("zever build: source checkout unavailable")

// ErrBuildCompile is returned when the slim `go build` fails.
var ErrBuildCompile = errors.New("zever build: compile failed")

// DefaultBuildOut is the default slim binary path for `zever build`.
const DefaultBuildOut = "zever-slim"

// maxBuildOutput is the most compiler output kept in a build error.
const maxBuildOutput = 2048

// adapterNameForDir maps an adapter module directory back to its adapter
// string, inverting adapterDirName: the password "argon2" dir serves the
// PHC-canonical "argon2id" name, the resilience "inproc" dir serves "memory",
// and the consolidated "db" dirs serve "db" (legacy per-backend picks are
// aliases callers resolve before reaching here).
func adapterNameForDir(battery, dir string) string {
	if battery == "password" && dir == "argon2" {
		return "argon2id"
	}

	if battery == "resilience" && dir == "inproc" {
		return "memory"
	}

	return dir
}

// slimAdaptersSource filters an adapters.go source to the selected battery
// set, returning gofmt-clean source. A nil selection keeps every adapter
// (the full CLI). Adapter imports whose module directory is unselected are
// dropped along with the cliProviders() entries referencing them; core/*
// and other non-adapter imports are always kept. Selection entries may be
// canonical names or legacy aliases (adapterDirName decides the directory).
// eltRefsDropped reports whether elt references any dropped adapter alias:
// a SelectorExpr X that names a dropped import alias (for example
// cachememory in cachememory.New) pins the element to that adapter.
func eltRefsDropped(elt ast.Expr, dropped map[string]bool) bool {
	refs := false

	ast.Inspect(elt, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		if dropped[id.Name] {
			refs = true

			return false
		}

		return true
	})

	return refs
}

func slimAdaptersSource(src string, sel []batterySelection) (string, error) {
	const tag = "zever build"

	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, "adapters.go", src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("%s: parse adapters source: %w", tag, err)
	}

	type adapterImport struct {
		alias   string
		battery string
		dir     string
	}

	byAlias := map[string]adapterImport{}

	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}

		for _, spec := range gen.Specs {
			imp, ok := spec.(*ast.ImportSpec)
			if !ok {
				continue
			}

			path, pathErr := strconv.Unquote(imp.Path.Value)
			if pathErr != nil {
				continue
			}

			rest, ok := strings.CutPrefix(path, "github.com/zenta-dev/zever/adapters/")
			if !ok {
				continue
			}

			battery, dir, ok := strings.Cut(rest, "/")
			if !ok || battery == "" || dir == "" {
				continue
			}

			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
			}

			byAlias[alias] = adapterImport{alias: alias, battery: battery, dir: dir}
		}
	}

	want := map[batterySelection]bool{}
	for _, b := range sel {
		want[b] = true
	}

	keptAlias := map[string]bool{}

	for alias, imp := range byAlias {
		if sel == nil {
			keptAlias[alias] = true

			continue
		}

		canonical := batterySelection{Battery: imp.battery, Adapter: adapterNameForDir(imp.battery, imp.dir)}
		if want[canonical] {
			keptAlias[alias] = true

			continue
		}

		for b := range want {
			if b.Battery == imp.battery && adapterDirName(b) == imp.battery+"/"+imp.dir {
				keptAlias[alias] = true

				break
			}
		}
	}

	dropped := map[string]bool{}

	for alias := range byAlias {
		if !keptAlias[alias] {
			dropped[alias] = true
		}
	}

	// Drop unselected adapter import specs, removing emptied blocks.
	kept := f.Decls[:0]

	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			kept = append(kept, decl)

			continue
		}

		specs := gen.Specs[:0]

		for _, spec := range gen.Specs {
			imp, ok := spec.(*ast.ImportSpec)
			if !ok {
				specs = append(specs, spec)

				continue
			}

			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
			}

			if _, isAdapter := byAlias[alias]; isAdapter && alias != "" && dropped[alias] {
				continue
			}

			specs = append(specs, spec)
		}

		if len(specs) == 0 {
			continue
		}

		gen.Specs = specs
		kept = append(kept, decl)
	}

	f.Decls = kept

	// Drop cliProviders() entries referencing dropped adapter aliases. The
	// providers live as elements of the returned slice literal, so filter
	// elements rather than statements: an element survives only when none
	// of its adapter aliases was dropped.
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "cliProviders" || fn.Body == nil {
			continue
		}

		for _, stmt := range fn.Body.List {
			ret, ok := stmt.(*ast.ReturnStmt)
			if !ok {
				continue
			}

			for _, res := range ret.Results {
				lit, ok := res.(*ast.CompositeLit)
				if !ok {
					continue
				}

				elts := lit.Elts[:0]

				for _, elt := range lit.Elts {
					if eltRefsDropped(elt, dropped) {
						continue
					}

					elts = append(elts, elt)
				}

				lit.Elts = elts
			}
		}
	}

	// Drop imports no kept code references anymore (unselected adapters and
	// the core packages whose Provide entries left with them).
	refs := map[string]bool{}

	for _, decl := range f.Decls {
		ast.Inspect(decl, func(n ast.Node) bool {
			gen, ok := n.(*ast.GenDecl)
			if ok && gen.Tok == token.IMPORT {
				return false
			}

			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			id, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			refs[id.Name] = true

			return true
		})
	}

	kept = f.Decls[:0]

	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			kept = append(kept, decl)

			continue
		}

		specs := gen.Specs[:0]

		for _, spec := range gen.Specs {
			imp, ok := spec.(*ast.ImportSpec)
			if !ok {
				specs = append(specs, spec)

				continue
			}

			path, pathErr := strconv.Unquote(imp.Path.Value)
			if pathErr != nil {
				specs = append(specs, spec)

				continue
			}

			name := ""
			if imp.Name != nil {
				name = imp.Name.Name
			} else if i := strings.LastIndex(path, "/"); i != -1 {
				name = path[i+1:]
			}

			if name == "_" || name == "." || name == "" || refs[name] {
				specs = append(specs, spec)

				continue
			}
		}

		if len(specs) == 0 {
			continue
		}

		gen.Specs = specs
		kept = append(kept, decl)
	}

	f.Decls = kept

	var buf bytes.Buffer

	if nodeErr := format.Node(&buf, fset, f); nodeErr != nil {
		return "", fmt.Errorf("%s: render slim adapters source: %w", tag, nodeErr)
	}

	clean, formatErr := format.Source(buf.Bytes())
	if formatErr != nil {
		return "", fmt.Errorf("%s: format slim adapters source: %w", tag, formatErr)
	}

	return string(clean), nil
}

// buildData is the --json payload for `zever build`.
type buildData struct {
	// Out is the output binary path.
	Out string `json:"out"`
	// SizeBytes is the built binary size (0 on dry runs).
	SizeBytes int64 `json:"sizeBytes"`
	// Adapters lists included battery/adapter pairs, sorted.
	Adapters []string `json:"adapters"`
	// DryRun reports a plan-only run.
	DryRun bool `json:"dryRun,omitempty"`
}

// buildOptions carries every input runBuildWith needs. Stdout defaults to
// os.Stdout, SourceDir defaults to this source file's directory, and Build
// defaults to exec'ing `go build`, so tests inject fakes without touching
// globals or the compiler.
type buildOptions struct {
	adapters  string
	out       string
	dryRun    bool
	json      bool
	stdout    io.Writer
	sourceDir string
	build     func(ctx context.Context, dir string, args []string) ([]byte, error)
}

// buildAdapterNames renders sel as sorted battery/adapter strings; a nil
// selection expands to every known pair.
func buildAdapterNames(sel []batterySelection) []string {
	if sel == nil {
		all := batteryAdapters()
		names := []string{}

		for battery, adapters := range all {
			for _, a := range adapters {
				names = append(names, battery+"/"+a)
			}
		}

		sort.Strings(names)

		return names
	}

	names := make([]string, 0, len(sel))
	for _, b := range sel {
		names = append(names, b.Battery+"/"+b.Adapter)
	}

	return names
}

// runBuildWith resolves --adapters, generates the slim adapters.go, and
// compiles the CLI via `go build -overlay` so unselected adapter packages
// (and their transitive dependencies) leave the binary. A nil selection
// builds the full CLI with a plain `go build`.
func runBuildWith(ctx context.Context, opts buildOptions) error {
	const tag = "zever build"

	sel, err := resolveBuildAdapters(opts.adapters)
	if err != nil {
		return err
	}

	stdout := opts.stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	out := opts.out
	if out == "" {
		out = DefaultBuildOut
	}

	names := buildAdapterNames(sel)
	data := buildData{Out: out, Adapters: names}

	if opts.dryRun {
		data.DryRun = true

		if opts.json {
			emitSuccess("build", data)

			return nil
		}

		if sel == nil {
			_, _ = fmt.Fprintf(stdout, "%s: dry-run: would build %s with the full adapter set (%d adapters)\n", tag, out, len(names))
		} else {
			_, _ = fmt.Fprintf(stdout, "%s: dry-run: would build %s with %d adapters: %s\n", tag, out, len(names), strings.Join(names, ", "))
		}

		return nil
	}

	dir := opts.sourceDir
	if dir == "" {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			return fmt.Errorf("%w: caller unavailable", ErrBuildSource)
		}

		dir = filepath.Dir(file)
	}

	orig := filepath.Join(dir, "adapters.go")

	src, err := os.ReadFile(orig)
	if err != nil {
		return errors.Join(fmt.Errorf("%w: read %s", ErrBuildSource, orig), err)
	}

	slim, err := slimAdaptersSource(string(src), sel)
	if err != nil {
		return err
	}

	build := opts.build
	if build == nil {
		build = func(ctx context.Context, dir string, args []string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, "go", args...)
			cmd.Dir = dir

			return cmd.CombinedOutput()
		}
	}

	outAbs := out
	if !filepath.IsAbs(outAbs) {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return fmt.Errorf("%s: resolve out path: %w", tag, cwdErr)
		}

		outAbs = filepath.Join(cwd, outAbs)
	}

	args := []string{"build", "-o", outAbs, "."}

	if sel != nil {
		tmp, tmpErr := os.MkdirTemp("", "zever-build")
		if tmpErr != nil {
			return fmt.Errorf("%s: temp dir: %w", tag, tmpErr)
		}

		defer func() { _ = os.RemoveAll(tmp) }()

		slimPath := filepath.Join(tmp, "adapters.go")
		if writeErr := os.WriteFile(slimPath, []byte(slim), 0o600); writeErr != nil {
			return fmt.Errorf("%s: write slim adapters: %w", tag, writeErr)
		}

		overlayPath := filepath.Join(tmp, "overlay.json")
		overlay := fmt.Sprintf("{\"Replace\":{%q:%q}}\n", orig, slimPath)

		if writeErr := os.WriteFile(overlayPath, []byte(overlay), 0o600); writeErr != nil {
			return fmt.Errorf("%s: write overlay: %w", tag, writeErr)
		}

		args = []string{"build", "-overlay", overlayPath, "-o", outAbs, "."}
	}

	if combined, buildErr := build(ctx, dir, args); buildErr != nil {
		msg := string(combined)
		if len(msg) > maxBuildOutput {
			msg = msg[len(msg)-maxBuildOutput:]
		}

		return errors.Join(fmt.Errorf("%w: %s", ErrBuildCompile, strings.TrimSpace(msg)), buildErr)
	}

	fi, err := os.Stat(outAbs)
	if err != nil {
		return fmt.Errorf("%s: stat %s: %w", tag, outAbs, err)
	}

	data.SizeBytes = fi.Size()

	if opts.json {
		emitSuccess("build", data)

		return nil
	}

	if sel == nil {
		_, _ = fmt.Fprintf(stdout, "%s: built %s (%d bytes, full adapter set: %d adapters); smoke-check with %s doctor\n", tag, out, fi.Size(), len(names), out)
	} else {
		_, _ = fmt.Fprintf(stdout, "%s: built %s (%d bytes, %d adapters: %s); smoke-check with %s doctor\n", tag, out, fi.Size(), len(names), strings.Join(names, ", "), out)
	}

	return nil
}

// newBuildFlagSet builds the stdlib flag set for `zever build`: the single
// source for runBuild parsing and help, so help can never drift.
func newBuildFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.String("adapters", "", "comma-separated batteries (bare name selects every adapter) or battery/adapter pins for the slim binary; empty builds the full CLI")
	fs.String("out", DefaultBuildOut, "output binary path")
	fs.Bool("dry-run", false, "print the adapter plan without compiling")

	return fs
}

// printBuildUsage prints styled help for `zever build`.
func printBuildUsage(fs *flag.FlagSet) {
	header := title("zever build") + dim(" — build a slim CLI with only the adapters you need")
	usage := bold("Usage:") + "  " + cmd("zever build") + dim(" [--adapters a,b] [--out PATH] [--dry-run]")

	body := joinLines(
		header,
		"",
		usage,
		"",
		"Compiles this CLI from the current source checkout, registering",
		"only the selected adapters: unselected adapter packages (and",
		"their transitive dependencies) leave the binary via a",
		"`go build -overlay` of the central registration list",
		"(adapters.go). An empty --adapters builds the full CLI.",
		"",
		"Each entry is a bare battery (every known adapter for it) or a",
		"battery/adapter pin; unknown names fail listing the choices.",
		"Smoke-check the result with `<out> doctor`.",
		"",
		bold("Flags:"),
	)
	_, _ = fmt.Fprintln(fs.Output(), body)
	printFlagDefaults(fs)
	_, _ = fmt.Fprintln(fs.Output(), "")
	_, _ = fmt.Fprintln(fs.Output(), dim("Examples:"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever build --adapters=log,router --out /tmp/zever-slim"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever build --adapters=cache/redis,queue/redis --dry-run"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(fs.Output(), formatHint("run '/tmp/zever-slim doctor' after building"))
	}
}

// runBuild parses argv and compiles the slim CLI.
func runBuild(args []string) error {
	fs := newBuildFlagSet()
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		return err
	}

	if len(fs.Args()) > 0 {
		return fmt.Errorf("zever build: unexpected positional %q", fs.Args()[0])
	}

	adapters := fs.Lookup("adapters").Value.String()
	out := fs.Lookup("out").Value.String()
	dryRun := fs.Lookup("dry-run").Value.String() == "true"

	return runBuildWith(context.Background(), buildOptions{
		adapters: adapters,
		out:      out,
		dryRun:   dryRun,
		json:     jsonMode,
	})
}

// newBuildCmd builds the `build` Cobra command, wrapping runBuild without
// touching its stdlib flag parsing: DisableFlagParsing passes raw args
// (including -h) straight through so the usage printer stays authoritative.
func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build [--adapters a,b] [--out PATH] [--dry-run]",
		Short: "Build a slim CLI with only the adapters you need",
		Long: `Build a slim zever CLI from the current source checkout,
registering only the selected adapters.`,
		Example: `  zever build --adapters=log,router --out /tmp/zever-slim
  zever build --adapters=cache/redis,queue/redis --dry-run`,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if herr := printHelpIfRequested(cmd, args); herr != nil {
				if errors.Is(herr, errHelpShown) {
					return nil
				}

				return herr
			}

			if versionRequested(cmd) {
				printCLIVersion()

				return nil
			}

			return runBuild(args)
		},
	}
	cmd.Flags().String("adapters", "", "comma-separated batteries or battery/adapter pins; empty builds the full CLI")
	cmd.Flags().String("out", DefaultBuildOut, "output binary path")
	cmd.Flags().Bool("dry-run", false, "print the adapter plan without compiling")

	return cmd
}
