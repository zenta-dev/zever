package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/backend"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
	"github.com/zenta-dev/zever/internal/dsl/backend/gogen"
	"github.com/zenta-dev/zever/internal/dsl/backend/openapi"
	"github.com/zenta-dev/zever/internal/dsl/backend/proto"
	"github.com/zenta-dev/zever/internal/dsl/backend/protogogen"
	"github.com/zenta-dev/zever/internal/dsl/backend/zenorm"
	"github.com/zenta-dev/zever/internal/dsl/compile"
	"github.com/zenta-dev/zever/internal/dsl/diag"
)

// errNoInputFiles is returned when neither explicit args nor auto-discovery
// yields a .zen file.
var errNoInputFiles = errors.New("zever compile: no input .zen files given")

// compilePromptSeams isolate huh prompts for tests.
// Proof: default values are the production prompt functions, so reachable
// behavior is identical; tests override them to simulate TTY selections
// without a real terminal.
var (
	promptMultiSelectFn = promptMultiSelect
	promptInputFn       = func(title, placeholder string) (string, error) {
		return promptInput(title, placeholder, nil)
	}
	discoverZenFilesFn = discoverZenFiles
)

//nolint:unused
const compileUsage = `zever compile [--backend atlas,gogen,openapi,proto,protogogen,zenorm] [--out DIR] <files...>

Compiles .zen schema files through one or more backends, writing each
backend's output under <out>/<backend>/.

Flags:`

// CompileConfig carries every input runCompileWith needs. Screen agents
// build it from huh forms; the flag shell (runCompile) builds it from argv.
// A nil Out defaults to os.Stdout. An empty Backends selects every
// registered backend.
type CompileConfig struct {
	Files    []string
	Backends string
	OutDir   string
	Out      io.Writer
}

// outOrStdout resolves the effective stdout writer for a command config.
func outOrStdout(w io.Writer) io.Writer {
	if w == nil {
		return os.Stdout
	}

	return w
}

// backendRoots carries the calling project's own module path and output
// layout, computed once per `zever compile` invocation (see
// computeBackendRoots), so a backend that generates code importing another
// backend's output can point at where this project actually puts it
// instead of a framework-repo-only default that's wrong for every other
// project. Both fields are "" when no local go.mod is found (e.g. schema
// files compiled from outside any Go module), which every consumer treats
// as "use my own hardcoded default" -- see gogen.NewWithPBImportRoot and
// proto.NewWithAnnotationsGoPackageRoot, both of which fall back on "".
type backendRoots struct {
	// pbImportRoot is where this project's protogogen output root lives as
	// a Go import path, e.g. "api/generated/protogogen" -- gogen's
	// generated application-layer code imports
	// "<pbImportRoot>/<module>" (or bare pbImportRoot for the implicit
	// module) for the real protobuf message types protogogen generated for
	// the same schema, matching protogogen's own "paths=source_relative"
	// output layout.
	pbImportRoot string
	// annotationsGoPackageRoot is the Go import path of this project's own
	// locally-compiled copy of the shared wire-stable annotations.proto
	// companion file. The .proto import path stays "zever/annotations.proto"
	// by design (wire-stable: every generated .proto imports it verbatim,
	// and the ported backends kept that path), so only the Go package root
	// is project-dependent: protogogen's "paths=source_relative" output
	// mirrors the .proto file's own path exactly, producing
	// "zever/annotations.pb.go" (a file in a directory named "zever", NOT
	// a directory named "zever/annotations" -- Go packages are addressed
	// by directory, so the import path is "<pbImportRoot>/zever", one
	// segment shorter than the .proto file's own name would suggest.
	annotationsGoPackageRoot string
}

// computeBackendRoots derives backendRoots from the local go.mod (if any)
// and outDir, the same --out value zever compile is about to write
// protogogen's (and every other backend's) output under.
func computeBackendRoots(outDir string) backendRoots {
	modulePath := modulePathOf("go.mod")
	if modulePath == "" {
		return backendRoots{}
	}

	rel := filepath.ToSlash(filepath.Clean(outDir))
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")

	pbRoot := modulePath + "/" + rel + "/protogogen"

	return backendRoots{
		pbImportRoot:             pbRoot,
		annotationsGoPackageRoot: pbRoot + "/zever",
	}
}

// backendRegistry maps a --backend name to a constructor for that
// backend.Backend, given the calling project's computeBackendRoots result
// -- ignored by every backend except gogen and proto, which need it to
// import/reference this project's own generated output instead of a
// zev-repo-only default. Adding a backend is a one-line addition here.
var backendRegistry = map[string]func(backendRoots) backend.Backend{
	"atlas":   func(backendRoots) backend.Backend { return atlas.New() },
	"gogen":   func(r backendRoots) backend.Backend { return gogen.NewWithPBImportRoot(r.pbImportRoot) },
	"openapi": func(backendRoots) backend.Backend { return openapi.New() },
	"proto": func(r backendRoots) backend.Backend {
		return proto.NewWithAnnotationsGoPackageRoot(r.annotationsGoPackageRoot)
	},
	"protogogen": func(r backendRoots) backend.Backend {
		return protogogen.NewWithAnnotationsGoPackageRoot(r.annotationsGoPackageRoot)
	},
	"zenorm": func(backendRoots) backend.Backend { return zenorm.New() },
}

// backendNames returns the registered backend names, sorted, for help and
// error text.
func backendNames() string {
	return strings.Join(backendNamesSlice(), ", ")
}

// defaultBackends returns the sorted backend names joined by "," for use as
// the --backend flag default. It includes every key in backendRegistry so
// future backends are auto-included.
func defaultBackends() string {
	return strings.Join(backendNamesSlice(), ",")
}

func backendNamesSlice() []string {
	names := make([]string, 0, len(backendRegistry))
	for name := range backendRegistry {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func runCompile(args []string) error { //nolint:gocyclo
	args = peelInteractive(args)
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	backendsFlag := fs.String("backend", defaultBackends(), "comma-separated list of backends to run (available: "+backendNames()+")")
	outDir := fs.String("out", "./generated", "output directory for generated files")
	// coverageProof: no local -i/--interactive flags; peelInteractive
	// strips them before Parse and sets interactiveMode globally.

	fs.Usage = func() {
		printCompileUsage(fs)
	}

	posArgs, err := flexibleParse(fs, args)
	if err != nil {
		return err
	}

	var paths []string

	if len(posArgs) == 0 && isInteractiveTerminal() {
		// -i was requested with a TTY attached: let the user hand-pick from
		// discovered files rather than silently using all of them.
		discovered := discoverZenFilesFn()
		if len(discovered) > 0 {
			sel, err2 := promptMultiSelectFn("Select schema files", discovered)
			if err2 == nil && len(sel) > 0 {
				paths = sel
			}
		}

		if len(paths) == 0 {
			// No schema dir at all (or nothing selected) -- fall back to a
			// manual prompt.
			val, err2 := promptInputFn("Schema files (space-separated)", "schema/app.zen")
			if err2 == nil && val != "" {
				paths = strings.Fields(val)
			}
		}
	}

	if len(paths) == 0 {
		var err2 error

		paths, err2 = resolveInputFiles(posArgs)
		if err2 != nil {
			if errors.Is(err2, errNoInputFiles) && shouldShowHint() {
				_, _ = fmt.Fprintln(os.Stderr, formatHint("try 'zever compile schema/app.zen --backend=proto,zenorm'"))
			}

			return err2
		}

		if len(posArgs) == 0 {
			reportAutoDiscovery(paths)
		}
	}

	return runCompileWith(CompileConfig{
		Files:    paths,
		Backends: *backendsFlag,
		OutDir:   *outDir,
	})
}

// runCompileWith compiles cfg.Files through the requested backends (one
// shared resolved schema, a single compile.WithSchemaDir call) and writes
// each backend's output under cfg.OutDir/<backend>/. It prints diagnostics
// to stderr and a success summary to cfg.Out.
func runCompileWith(cfg CompileConfig) error {
	backendsSpec := cfg.Backends
	if strings.TrimSpace(backendsSpec) == "" {
		backendsSpec = defaultBackends()
	}

	outDir := cfg.OutDir
	if outDir == "" {
		outDir = "./generated"
	}

	out := outOrStdout(cfg.Out)

	files, err := loadFiles(cfg.Files)
	if err != nil {
		return err
	}

	backends, err := resolveBackends(backendsSpec, computeBackendRoots(outDir))
	if err != nil {
		if shouldShowHint() {
			if s := closest(strings.TrimSpace(strings.Split(backendsSpec, ",")[0]), allBackends); s != "" {
				_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
			}
		}

		return err
	}

	schemaDir := defaultSchemaDir
	if pc, err := loadProjectConfig(); err == nil {
		schemaDir = pc.SchemaDir
	}

	result, diags := compile.WithSchemaDir(files, schemaDir, backends...)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	if diags.HasErrors() {
		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("fix diagnostics above, then re-run compile"))
		}

		return fmt.Errorf("zever compile: %d file(s) failed to compile", len(files))
	}

	if err := writeOutputs(outDir, result.Outputs); err != nil {
		return err
	}

	// Success summary.
	total := 0
	for _, m := range result.Outputs {
		total += len(m)
	}

	msg := success("✔ compiled ") + bold(fmt.Sprintf("%d file(s)", len(files))) + dim(" → ") +
		cyan(outDir) + dim(fmt.Sprintf(" (%d outputs, backends: %s)", total, backendsSpec))
	_, _ = fmt.Fprintln(out, msg)

	if shouldShowHint() {
		if h := hintFor("compile"); h != "" {
			_, _ = fmt.Fprintln(os.Stderr, formatHint(h))
		}
	}

	return nil
}

func printCompileUsage(fs *flag.FlagSet) {
	header := title("zever compile") + dim(" — schema → generated")
	flags := dim(" [--backend " + defaultBackends() + "] [--out DIR]")
	usage := bold("Usage:") + "  " + cmd("zever compile") + flags + "  " + cyan("<files...>")
	body := joinLines(
		header,
		"",
		usage,
		"",
		bold("Flags:"),
		dim("Available backends: ")+cyan(backendNames()),
	)
	_, _ = fmt.Fprintln(fs.Output(), body)
	fs.PrintDefaults()
	_, _ = fmt.Fprintln(fs.Output(), "")
	_, _ = fmt.Fprintln(fs.Output(), dim("Examples:"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever compile schema/app.zen --backend=proto"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever compile --backend=zenorm,proto,atlas,openapi schema/*.zen --out ./generated"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever compile -i")+dim("  # guided: select files & backends"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(fs.Output(), formatHint("add -i for guided prompts"))
	}
}

// resolveBackends parses a comma-separated --backend value into concrete
// backend.Backend instances via backendRegistry, constructed with roots.
func resolveBackends(spec string, roots backendRoots) ([]backend.Backend, error) {
	names := strings.Split(spec, ",")
	backends := make([]backend.Backend, 0, len(names))

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		ctor, ok := backendRegistry[name]
		if !ok {
			if s := closest(name, allBackends); s != "" {
				return nil, fmt.Errorf("zever compile: unknown backend %q (did you mean %q? available: %s)", name, s, backendNames())
			}

			return nil, fmt.Errorf("zever compile: unknown backend %q (available: %s)", name, backendNames())
		}

		backends = append(backends, ctor(roots))
	}

	return backends, nil
}

// loadFiles reads each positional path into a map keyed by that path, the
// shape compile.Compile expects.
func loadFiles(paths []string) (map[string]string, error) {
	if len(paths) == 0 {
		return nil, errNoInputFiles
	}

	files := make(map[string]string, len(paths))

	for _, path := range paths {
		data, err := os.ReadFile(path) //nolint:gosec // CLI positional args are developer-supplied file paths
		if err != nil {
			return nil, fmt.Errorf("zever compile: read %q: %w", path, err)
		}

		files[path] = string(data)
	}

	return files, nil
}

// printDiagnostics writes every diagnostic to stderr in sorted, human-readable order with colors.
func printDiagnostics(diags diag.List) {
	for _, d := range diags.Sorted() {
		msg := d.Error()
		// Color by severity if present.
		switch d.Severity {
		case diag.SeverityError:
			msg = red(failMark() + " " + msg)
		case diag.SeverityWarning:
			msg = yellow("⚠ " + msg)
		default:
			msg = dim(msg)
		}

		_, _ = fmt.Fprintln(os.Stderr, msg)
	}
}

// writeOutputs writes result.Outputs[backend][file] bytes to
// <outDir>/<backend>/<file>, creating directories as needed.
func writeOutputs(outDir string, outputs map[string]map[string][]byte) error {
	for backendName, files := range outputs {
		for name, content := range files {
			dest := filepath.Join(outDir, backendName, name)

			if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
				return fmt.Errorf("zever compile: mkdir %q: %w", filepath.Dir(dest), err)
			}

			if err := os.WriteFile(dest, content, 0o644); err != nil { //nolint:gosec // generated source output, not a secret
				return fmt.Errorf("zever compile: write %q: %w", dest, err)
			}
		}
	}

	return nil
}
