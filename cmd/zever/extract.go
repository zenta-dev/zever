package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/backend/zenorm"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

const extractUsageBody = `Extracts one schema module into a standalone, separately deployable Go module:
its own go.mod, a verbatim copy of the .zen file(s) that declare it, the
generated ORM package for just its entities, and cmd/server + cmd/worker
entrypoints wired to only that module's jobs and schedules.

This is safe to do because the resolver already enforces hard module
isolation: a relation, an RPC return type or a permission-check resource may
never cross a module boundary, so a module's declarations genuinely do not
reference another module's data. Extraction relocates schema-derived
scaffolding, it does not rewrite any code.

Every .zen file under <schema_dir> is compiled first, so the target module is
resolved in the context of the whole workspace. The schema file(s) that
declare it are then copied byte for byte -- there is no .zen printer, and
re-serializing from the AST would lose comments and formatting. A file that
declares two different modules therefore cannot be extracted; split it first.

Not automated, exactly as with ` + "`zever generate server`" + ` and
` + "`zever generate worker`" + `: route registration and handler bodies, and job
handler bodies. Those land as stub comments for you to fill in. Extraction
also writes no go.sum -- run ` + "`go mod tidy`" + ` in the output directory.`

//nolint:dupl
func printExtractUsage(fs *flag.FlagSet) {
	header := title("zever extract") + dim(" — module → standalone service")
	//nolint:lll
	usage := bold("Usage:") + "  " + cmd("zever extract") + dim(" <module> [--out DIR] [--module PATH] [--force]")

	body := joinLines(
		header,
		"",
		usage,
		"",
		extractUsageBody,
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
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever extract shop --out ./shop-service"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever extract shop --module example.com/shop-service --force"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(fs.Output(), formatHint("extraction is non-interactive; pass every value as a flag"))
	}
}

// ExtractConfig is the pure input to planExtraction/writeExtraction: what to
// extract and where to put it, resolved before anything is written, so a
// failure to work out the target leaves no half-written directory. The TUI
// screens (Wave 3) build this struct directly and call the same
// plan-then-write path; runExtract builds it from CLI flags. It performs no
// I/O itself.
type ExtractConfig struct {
	// Module is the schema module name to extract (required).
	Module string
	// OutDir is the output directory (default ./<module>-service).
	OutDir string
	// ModulePath is the module path for the extracted go.mod (default
	// <this module>/<module>-service).
	ModulePath string
	// Force overwrites files that already exist in the output directory.
	Force bool
}

// extractPlan is everything planExtraction resolves before anything is
// written, so that a failure to work out the target leaves no half-written
// directory.
type extractPlan struct {
	Module      *ir.Module
	SchemaFiles []string
	OutDir      string
	ModulePath  string
	Force       bool
}

// runExtract extracts one module from flags only. There is no interactive
// path here: no prompts, no TTY checks, no module discovery menu. Guided
// input lives in the TUI screens, which build an ExtractConfig and call the
// same plan-then-write path; this stays the non-interactive, flag-driven
// entry point.
func runExtract(args []string) error {
	const tag = "zever extract"

	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	outDir := fs.String("out", "", "output directory (default ./<module>-service)")
	modulePath := fs.String("module", "", "module path for the extracted go.mod (default <this module>/<module>-service)")
	force := fs.Bool("force", false, "overwrite files that already exist in the output directory")

	fs.Usage = func() {
		printExtractUsage(fs)
	}

	posArgs, err := flexibleParse(fs, args)
	if err != nil {
		return err
	}

	if len(posArgs) != 1 {
		fs.Usage()
		return fmt.Errorf("%s: expected exactly one module name, got %d", tag, len(posArgs))
	}

	cfg := ExtractConfig{Module: posArgs[0], OutDir: *outDir, ModulePath: *modulePath, Force: *force}

	project, err := loadProjectConfig()
	if err != nil {
		return err
	}

	gomod, err := readGoMod()
	if err != nil {
		return err
	}

	schema, err := compileSchemaDir(tag, project.SchemaDir)
	if err != nil {
		return err
	}

	plan, err := planExtraction(tag, cfg, schema, gomod)
	if err != nil {
		return err
	}

	return writeExtraction(tag, plan, gomod)
}

// planExtraction resolves the target module, the source files that declare it
// and the output location, without touching the filesystem.
func planExtraction(
	tag string,
	cfg ExtractConfig,
	schema *ir.Schema,
	gomod goModInfo,
) (extractPlan, error) {
	mod := findModule(schema, cfg.Module)
	if mod == nil {
		return extractPlan{}, fmt.Errorf("%s: no module %q in the schema (declared modules: %s)",
			tag, cfg.Module, strings.Join(moduleNames(schema), ", "))
	}

	files, err := moduleSourceFiles(tag, schema, mod)
	if err != nil {
		return extractPlan{}, err
	}

	outDir := cfg.OutDir
	if outDir == "" {
		outDir = "./" + cfg.Module + "-service"
	}

	modulePath := cfg.ModulePath
	if modulePath == "" {
		modulePath = gomod.ModulePath + "/" + cfg.Module + "-service"
	}

	return extractPlan{
		Module:      mod,
		SchemaFiles: files,
		OutDir:      outDir,
		ModulePath:  modulePath,
		Force:       cfg.Force,
	}, nil
}

// findModule looks a module up by exact name. The implicit unnamed module
// (a schema with no `module` blocks at all) is not extractable: there is
// nothing to extract it from.
func findModule(schema *ir.Schema, name string) *ir.Module {
	if name == "" {
		return nil
	}

	for _, m := range schema.Modules {
		if m.Name == name {
			return m
		}
	}

	return nil
}

func moduleNames(schema *ir.Schema) []string {
	names := make([]string, 0, len(schema.Modules))

	for _, m := range schema.Modules {
		if m.Name != "" {
			names = append(names, m.Name)
		}
	}

	sort.Strings(names)

	if len(names) == 0 {
		return []string{"(none)"}
	}

	return names
}

// moduleSourceFiles returns the .zen files that contributed a declaration to
// mod. A file that also declares another module is rejected: the extracted
// copy is verbatim, so it would carry that other module's declarations along
// with it and break the isolation the extraction exists to produce.
func moduleSourceFiles(tag string, schema *ir.Schema, mod *ir.Module) ([]string, error) {
	owners := map[string]map[string]bool{}

	for _, m := range schema.Modules {
		for _, file := range declaredFiles(m) {
			if owners[file] == nil {
				owners[file] = map[string]bool{}
			}

			owners[file][m.Name] = true
		}
	}

	files := declaredFiles(mod)
	if len(files) == 0 {
		return nil, fmt.Errorf("%s: module %q declares nothing to extract", tag, mod.Name)
	}

	for _, file := range files {
		if len(owners[file]) > 1 {
			others := make([]string, 0, len(owners[file]))

			for other := range owners[file] {
				if other != mod.Name {
					others = append(others, other)
				}
			}

			sort.Strings(others)

			return nil, fmt.Errorf(
				"%s: %q declares module %q alongside %s; move each module into its own file before extracting",
				tag, file, mod.Name, strings.Join(others, ", "))
		}
	}

	return files, nil
}

// declaredFiles returns every source file a module has a declaration in,
// sorted and de-duplicated.
//
// Adaptation note: zever's Module also carries Messages and Enums alongside
// Entities/Services/Jobs/Schedules; all six declaration kinds count as
// ownership, so a message-only file is extracted (or conflicts) correctly
// instead of being invisible to the ownership map.
func declaredFiles(m *ir.Module) []string {
	set := map[string]bool{}

	add := func(pos diag.Position) {
		if pos.File != "" {
			set[pos.File] = true
		}
	}

	add(m.Pos)

	for _, e := range m.Entities {
		add(e.Pos)
	}

	for _, e := range m.Enums {
		add(e.Pos)
	}

	for _, msg := range m.Messages {
		add(msg.Pos)
	}

	for _, s := range m.Services {
		add(s.Pos)
	}

	for _, j := range m.Jobs {
		add(j.Pos)
	}

	for _, s := range m.Schedules {
		add(s.Pos)
	}

	files := make([]string, 0, len(set))
	for file := range set {
		files = append(files, file)
	}

	sort.Strings(files)

	return files
}

// writeExtraction materialises a planned extraction on disk.
func writeExtraction(tag string, plan extractPlan, gomod goModInfo) error {
	written := make([]string, 0, len(plan.SchemaFiles)+6)

	copied, err := copySchemaFiles(tag, plan)
	if err != nil {
		return err
	}

	written = append(written, copied...)

	modFile, err := renderExtractedGoMod(tag, plan, gomod)
	if err != nil {
		return err
	}

	path := filepath.Join(plan.OutDir, "go.mod")

	if writeErr := writeScaffold(tag, path, modFile, plan.Force); writeErr != nil {
		return writeErr
	}

	written = append(written, path)

	ormFiles, err := writeExtractedORM(tag, plan)
	if err != nil {
		return err
	}

	written = append(written, ormFiles...)

	goFiles, err := writeExtractedGoFiles(tag, plan)
	if err != nil {
		return err
	}

	written = append(written, goFiles...)

	// app.go's DefaultDBPath is data/app.db; keeping the directory in the
	// tree means the scaffolded binaries run without a mkdir first.
	keep := filepath.Join(plan.OutDir, "data", ".gitkeep")

	if writeErr := writeScaffold(tag, keep, nil, true); writeErr != nil {
		return writeErr
	}

	printExtractSummary(plan, written)

	return nil
}

// copySchemaFiles copies the module's .zen sources into <out>/schema byte for
// byte. No AST reprint: the DSL has no printer, and a copy cannot lose
// comments or formatting.
func copySchemaFiles(tag string, plan extractPlan) ([]string, error) {
	written := make([]string, 0, len(plan.SchemaFiles))

	for _, src := range plan.SchemaFiles {
		data, err := os.ReadFile(src) //nolint:gosec // path came from the compiled schema's own positions
		if err != nil {
			return nil, fmt.Errorf("%s: read %q: %w", tag, src, err)
		}

		dest := filepath.Join(plan.OutDir, defaultSchemaDir, filepath.Base(src))

		if err := writeScaffold(tag, dest, data, plan.Force); err != nil {
			return nil, err
		}

		written = append(written, dest)
	}

	return written, nil
}

// writeExtractedORM runs the zenorm backend over a schema holding only the
// target module, then relocates its output under the extracted module.
//
// Adaptation note: zever's zenorm backend emits its output under
// orm/gen/<module>/<module>.go importing only
// github.com/zenta-dev/zever/orm -- resolved by the extracted module's
// replace directive -- so relocating the package under internal/orm is the
// whole of the step; no self-import rewrite is needed.
func writeExtractedORM(tag string, plan extractPlan) ([]string, error) {
	single := &ir.Schema{Modules: []*ir.Module{plan.Module}}

	outputs, err := zenorm.New().Generate(single)
	if err != nil {
		return nil, fmt.Errorf("%s: zenorm backend: %w", tag, err)
	}

	paths := make([]string, 0, len(outputs))
	for path := range outputs {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	ormRoot := filepath.Join(plan.OutDir, "internal", "orm")

	written := make([]string, 0, len(paths))

	for _, path := range paths {
		// path is "orm/gen/<pkg>/<pkg>.go"; keep everything after "orm/gen/".
		rel := strings.TrimPrefix(filepath.ToSlash(path), "orm/gen/")

		dest := filepath.Join(ormRoot, filepath.FromSlash(rel))

		// Defense in depth: moduleNaming already rejects a path-unsafe
		// module name, but this confirms the write itself never lands
		// outside ormRoot regardless of how dest was derived.
		if relCheck, err := filepath.Rel(ormRoot, dest); err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("%s: %w: %q", tag, ErrPathTraversal, path)
		}

		if err := writeScaffold(tag, dest, outputs[path], plan.Force); err != nil {
			return nil, err
		}

		written = append(written, dest)
	}

	return written, nil
}

// writeExtractedGoFiles scaffolds internal/app plus the two entrypoints,
// reusing the same templates `zever generate server` and
// `zever generate worker` render, but against a schema holding only the
// extracted module -- so the worker registers that module's jobs and
// schedules and nothing else.
func writeExtractedGoFiles(tag string, plan extractPlan) ([]string, error) {
	single := &ir.Schema{Modules: []*ir.Module{plan.Module}}
	workerData := buildWorkerData(plan.ModulePath, single)

	files := []struct {
		path string
		name string
		tmpl string
		data any
	}{
		{
			path: filepath.Join("internal", "app", "app.go"),
			name: "app.go",
			tmpl: appTemplate,
		},
		{
			path: filepath.Join(defaultServerEntry, "main.go"),
			name: "server main.go",
			tmpl: serverTemplate,
			data: serverData{ModulePath: plan.ModulePath},
		},
		{
			path: filepath.Join(defaultWorkerEntry, "main.go"),
			name: "worker main.go",
			tmpl: workerTemplate,
			data: workerData,
		},
	}

	written := make([]string, 0, len(files))

	for _, f := range files {
		content, err := renderGoFile(tag, f.name, f.tmpl, f.data)
		if err != nil {
			return nil, err
		}

		dest := filepath.Join(plan.OutDir, f.path)

		if err := writeScaffold(tag, dest, content, plan.Force); err != nil {
			return nil, err
		}

		written = append(written, dest)
	}

	jobStubsWritten, err := writeJobStubs(tag, plan.OutDir, workerData)
	if err != nil {
		return nil, err
	}

	written = append(written, jobStubsWritten...)

	return written, nil
}

// renderExtractedGoMod writes the standalone module's go.mod.
//
// The framework is wired in with a local replace directive whenever this
// project can point at a checkout of it -- the only thing that works while
// zever has no tagged release. When the project resolves zever from the
// module cache instead, the extracted go.mod simply requires the same
// version this project does.
func renderExtractedGoMod(tag string, plan extractPlan, gomod goModInfo) ([]byte, error) {
	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "module %s\n\ngo %s\n\n", plan.ModulePath, gomod.GoVersion)

	if gomod.FrameworkDir != "" {
		rel, err := relativeTo(plan.OutDir, gomod.FrameworkDir)
		if err != nil {
			return nil, fmt.Errorf("%s: locate the framework from %q: %w", tag, plan.OutDir, err)
		}

		_, _ = fmt.Fprintf(&b, "// Extracted services build against the framework checkout they were\n"+
			"// extracted from; there is no tagged zever release yet.\nreplace %s => %s\n\n",
			frameworkModulePath, rel)
	}

	_, _ = fmt.Fprintf(&b, "require %s %s\n", frameworkModulePath, gomod.FrameworkVersion)

	return []byte(b.String()), nil
}

// relativeTo returns target expressed relative to base, in the slash-separated
// "./x" or "../x" form a go.mod replace directive needs.
func relativeTo(base, target string) (string, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return "", err
	}

	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}

	return rel, nil
}

func printExtractSummary(plan extractPlan, written []string) {
	mod := plan.Module

	_, _ = fmt.Fprintln(os.Stdout, success("✔ extracted ")+bold(fmt.Sprintf("module %q", mod.Name))+dim(" into ")+cyan(plan.OutDir))
	_, _ = fmt.Fprintln(os.Stdout, "  "+dim("module path: ")+cyan(plan.ModulePath))
	_, _ = fmt.Fprintln(os.Stdout, "  "+dim(fmt.Sprintf("%d entities, %d services, %d jobs, %d schedules", len(mod.Entities), len(mod.Services), len(mod.Jobs), len(mod.Schedules))))

	for _, path := range written {
		_, _ = fmt.Fprintln(os.Stdout, "  "+dim("wrote ")+cyan(path))
	}

	next := joinLines(
		bold("Next steps:"),
		"  "+dim("1.")+"  "+cmd("cd "+plan.OutDir),
		"  "+dim("2.")+"  "+cmd("go mod tidy"),
		"  "+dim("3.")+"  "+cmd("go build ./..."),
	)

	_, _ = fmt.Fprintln(os.Stdout, "")

	if colorEnabled {
		_, _ = fmt.Fprintln(os.Stdout, box(next))
	} else {
		_, _ = fmt.Fprintln(os.Stdout, next)
	}

	_, _ = fmt.Fprintln(os.Stdout, dim("route registration and job handler bodies are left as stubs, as with ")+cyan("zever generate server")+dim("/")+cyan("worker")+dim("."))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(os.Stderr, formatHint(hintFor("extract")))
	}
}

// --- go.mod reading ---

// goModInfo is the little bit of ./go.mod an extraction needs: the current
// module path, its Go version, and how it resolves the framework.
type goModInfo struct {
	ModulePath       string
	GoVersion        string
	FrameworkVersion string
	FrameworkDir     string // local checkout, relative to the working directory, or ""
}

// readGoMod parses ./go.mod by hand. Only four facts are needed and the CLI
// has stayed dependency-free, so this stays a line scanner rather than a new
// dependency on golang.org/x/mod.
func readGoMod() (goModInfo, error) {
	f, err := os.Open("go.mod")
	if err != nil {
		if os.IsNotExist(err) {
			return goModInfo{}, errNoGoMod
		}

		return goModInfo{}, fmt.Errorf("zever extract: open go.mod: %w", err)
	}

	defer func() { _ = f.Close() }()

	info := goModInfo{GoVersion: defaultGoVersion, FrameworkVersion: pseudoVersionZero}
	scanner := bufio.NewScanner(f)

	var block string

	for scanner.Scan() {
		line, _, _ := strings.Cut(scanner.Text(), "//")

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if line == ")" {
			block = ""

			continue
		}

		block = scanGoModLine(&info, block, line)
	}

	if err := scanner.Err(); err != nil {
		return goModInfo{}, fmt.Errorf("zever extract: read go.mod: %w", err)
	}

	if info.ModulePath == "" {
		return goModInfo{}, errors.New("zever extract: go.mod declares no module path")
	}

	resolveFrameworkDir(&info)

	return info, nil
}

// scanGoModLine folds one go.mod line into info and returns the block context
// ("require", "replace" or "") the next line is read in.
func scanGoModLine(info *goModInfo, block, line string) string {
	switch {
	case strings.HasPrefix(line, "module "):
		info.ModulePath = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "module")), `"`)
	case strings.HasPrefix(line, "go "):
		info.GoVersion = strings.TrimSpace(strings.TrimPrefix(line, "go"))
	case line == "require (":
		return "require"
	case line == "replace (":
		return "replace"
	case strings.HasPrefix(line, "require "):
		readRequire(info, strings.TrimPrefix(line, "require "))
	case strings.HasPrefix(line, "replace "):
		readReplace(info, strings.TrimPrefix(line, "replace "))
	case block == "require":
		readRequire(info, line)
	case block == "replace":
		readReplace(info, line)
	}

	return block
}

func readRequire(info *goModInfo, line string) {
	fields := strings.Fields(line)
	if len(fields) >= 2 && fields[0] == frameworkModulePath {
		info.FrameworkVersion = fields[1]
	}
}

// readReplace records a local-path replacement of the framework. Only the
// filesystem-path form matters here: a module-to-module replacement is
// resolved by the module cache and needs no directive of its own.
func readReplace(info *goModInfo, line string) {
	left, right, ok := strings.Cut(line, "=>")
	if !ok {
		return
	}

	leftFields := strings.Fields(left)
	if len(leftFields) == 0 || leftFields[0] != frameworkModulePath {
		return
	}

	fields := strings.Fields(right)
	if len(fields) != 1 {
		return
	}

	target := fields[0]
	if strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../") || filepath.IsAbs(target) {
		info.FrameworkDir = target
	}
}

// resolveFrameworkDir fills in FrameworkDir for the case a replace directive
// cannot cover: this project *is* zever (its own examples and tools live
// inside the repository), so the framework checkout is the working directory.
func resolveFrameworkDir(info *goModInfo) {
	if info.FrameworkDir != "" || info.ModulePath != frameworkModulePath {
		return
	}

	info.FrameworkDir = "."
	info.FrameworkVersion = pseudoVersionZero
}
