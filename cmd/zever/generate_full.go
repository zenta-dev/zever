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

	"github.com/zenta-dev/zever/dsl/naming"
)

// GenerateFullConfig is the pure input to GenerateFull: the default
// `zever generate` pipeline (compile with the default backend set, stamp,
// write generated outputs, scaffold the server entrypoint and module
// wiring on top of the serve library). An empty Backends selects
// GenerateDefaultBackends (ZEVER_BACKENDS wins over the constant, an
// explicit flag over both); proto/protogogen join automatically when the
// schema declares services.
type GenerateFullConfig struct {
	Backends    string
	OutDir      string
	SchemaDir   string
	ServerEntry string
	ModulePath  string
	SchemaFiles []string
	// Diff prints the unified diff of generated files and writes nothing.
	Diff bool
	// Check exits non-zero on drift and writes nothing (the CI drift gate,
	// extending the `zever compile --check` pattern to the scaffold files).
	Check bool
	// Strict makes --check also fail when refreshed files differ only in
	// the stamped CLI version.
	Strict bool
	Stdout io.Writer
	Stderr io.Writer
}

// fileStatus names one generated file's disposition in a pipeline run.
type fileStatus struct {
	Path   string
	Status string // created, updated, unchanged, missing, differs
}

const (
	fileCreated   = "created"
	fileUpdated   = "updated"
	fileUnchanged = "unchanged"
)

// runGenerateFull is the `zever generate` bare pipeline entrypoint: no
// subcommand word, full regeneration with the default backend set.
func runGenerateFull(args []string) error {
	args = peelInteractive(args)
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("generate", flag.ContinueOnError)
		printGenerateFullUsage(fs)

		return nil
	}

	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	backends := fs.String("backend", "", "comma-separated backends (default "+GenerateDefaultBackends+", auto-adding proto,protogogen for service schemas; ZEVER_BACKENDS overrides the default)")
	outDir := fs.String("out", "./generated", "output directory for generated files")
	diff := fs.Bool("diff", false, "print the unified diff of generated files and write nothing")
	check := fs.Bool("check", false, "verify generated files match a fresh pipeline run (exit non-zero on drift, write nothing)")
	strict := fs.Bool("strict", false, "with --check, also fail when files differ only in the stamped CLI version")
	fs.Usage = func() {
		printGenerateFullUsage(fs)
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

	return GenerateFull(GenerateFullConfig{
		Backends:    *backends,
		OutDir:      *outDir,
		SchemaDir:   project.SchemaDir,
		ServerEntry: project.ServerEntry,
		ModulePath:  modulePath,
		SchemaFiles: posArgs,
		Diff:        *diff,
		Check:       *check,
		Strict:      *strict,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	})
}

func printGenerateFullUsage(fs *flag.FlagSet) {
	header := title("zever generate") + dim(" — full pipeline")
	usage := bold("Usage:") + "  " + cmd("zever generate") + dim(" [--backend SET] [--out DIR] [--diff] [--check] [files...]")
	desc := dim("Compiles .zen schemas with the default backend set, stamps and writes") + "\n" +
		dim("every generated file, then refreshes the serve-based server scaffold.") + "\n" +
		dim("No subcommand: this is the one command a fresh clone runs.")

	body := joinLines(
		header,
		"",
		usage,
		"",
		desc,
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
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever generate")+dim("                  # compile + scaffold with defaults"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever generate --diff")+dim("             # preview without writing"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever generate --check")+dim("            # CI drift gate"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever generate --backend=gogen,zenorm")+dim(" # explicit set, no auto-add"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(fs.Output(), formatHint("subcommands (module, entity, server, ...) still work: zever generate server -h"))
	}
}

// GenerateFull runs the default pipeline: compile, stamp, then either
// diff, check, or write + scaffold. It performs no flag parsing.
func GenerateFull(cfg GenerateFullConfig) error { //nolint:gocyclo
	const tag = "zever generate"

	outDir := cfg.OutDir
	if outDir == "" {
		outDir = "./generated"
	}

	serverEntry := cfg.ServerEntry
	if serverEntry == "" {
		serverEntry = defaultServerEntry
	}

	schemaDir := cfg.SchemaDir
	if schemaDir == "" {
		schemaDir = defaultSchemaDir
	}

	stdout := cfg.Stdout
	if stdout == nil {
		stdout = io.Discard
	}

	paths, err := resolveInputFiles(cfg.SchemaFiles)
	if err != nil && !errors.Is(err, errNoInputFiles) {
		return fmt.Errorf("%s: %w", tag, err)
	}

	if len(cfg.SchemaFiles) == 0 && err == nil {
		reportAutoDiscovery(paths)
	}

	var files map[string]string
	if err == nil {
		loaded, loadErr := loadFiles(paths)
		if loadErr != nil {
			return fmt.Errorf("%s: %w", tag, loadErr)
		}

		files = loaded
	}

	var stamped map[string]map[string][]byte

	var finalSpec string

	if files != nil {
		result, spec, diags, compileErr := compileSchemaWithDefaults(files, schemaDir, cfg.Backends, outDir)
		if compileErr != nil {
			if shouldShowHint() {
				if s := closest(strings.TrimSpace(strings.Split(spec, ",")[0]), allBackends); s != "" {
					_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
				}
			}

			return compileErr
		}

		finalSpec = spec

		if len(diags) > 0 {
			printDiagnosticsWithSources(diags, files)
		}

		if diags.HasErrors() {
			if shouldShowHint() {
				_, _ = fmt.Fprintln(os.Stderr, formatHint("fix diagnostics above, then re-run generate"))
			}

			return fmt.Errorf("%s: %d file(s) failed to compile", tag, len(files))
		}

		stamped = stampGeneratedOutputs(files, result.Outputs)
	} else {
		finalSpec, _ = resolveBackendSpec(cfg.Backends)
	}

	if _, appPkgErr := ensureAppPackage(tag); appPkgErr != nil {
		return appPkgErr
	}

	data, err := loadServerData(cfg.ModulePath, outDir, schemaDir, cfg.SchemaFiles, rateLimitConfigured(), reflectionEnabled())
	if err != nil {
		return err
	}

	mainContent, err := renderGoFile(tag, "server main.go", serverTemplate, data)
	if err != nil {
		return err
	}

	modulesContent, err := renderModulesContent(tag, data)
	if err != nil {
		return err
	}

	desired := map[string][]byte{
		filepath.Join(serverEntry, "main.go"):             mainContent,
		filepath.Join("internal", "app", modulesFileName): modulesContent,
	}

	scaffold := map[string]bool{}
	for path := range desired {
		scaffold[path] = true
	}

	for backendName, backendFiles := range stamped {
		for name, content := range backendFiles {
			desired[filepath.Join(outDir, backendName, name)] = content
		}
	}

	statuses := statDesiredTree(desired)

	switch {
	case cfg.Diff:
		printUnifiedDiffs(stdout, desired, statuses)

		return nil
	case cfg.Check:
		return reportGenerateCheck(cfg, stdout, finalSpec, desired, statuses, data, scaffold)
	default:
		return writeGenerateTree(tag, stdout, cfg.Stderr, outDir, finalSpec, desired, statuses, data)
	}
}

// statDesiredTree compares every desired file against disk without writing.
func statDesiredTree(desired map[string][]byte) []fileStatus {
	statuses := make([]fileStatus, 0, len(desired))

	for path, content := range desired {
		disk, err := os.ReadFile(path)
		switch {
		case err != nil:
			statuses = append(statuses, fileStatus{Path: path, Status: fileCreated})
		case len(disk) == len(content) && string(disk) == string(content):
			statuses = append(statuses, fileStatus{Path: path, Status: fileUnchanged})
		default:
			statuses = append(statuses, fileStatus{Path: path, Status: fileUpdated})
		}
	}

	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Path < statuses[j].Path })

	return statuses
}

// stubDrift returns the service stubs a fresh pipeline would create but that
// are missing from disk. Existing stubs never drift (HARD RULE R1: they are
// hand-owned once created), so only absence is reported.
func stubDrift(data serverData) []fileStatus {
	var missing []fileStatus

	for _, mod := range data.Modules {
		for _, svc := range mod.Services {
			path := filepath.Join(mod.ImplDir, serviceStubFileName(svc.Name))
			if _, err := os.Stat(path); os.IsNotExist(err) {
				missing = append(missing, fileStatus{Path: path, Status: fileCreated})
			}
		}
	}

	sort.Slice(missing, func(i, j int) bool { return missing[i].Path < missing[j].Path })

	return missing
}

// serviceStubFileName is the stub file name for a service: snake_case plus
// .go, the same rule writeServiceStubs uses.
func serviceStubFileName(service string) string {
	return naming.SnakeCase(service) + ".go"
}

// printUnifiedDiffs prints one unified diff per changed file: a single hunk
// with three lines of context around the trimmed common prefix/suffix.
// Missing files print as full adds. Unchanged files print nothing.
func printUnifiedDiffs(out io.Writer, desired map[string][]byte, statuses []fileStatus) {
	byPath := make(map[string][]byte, len(desired))
	for path, content := range desired {
		byPath[path] = content
	}

	for _, st := range statuses {
		if st.Status == fileUnchanged {
			continue
		}

		var old []byte

		if st.Status == fileUpdated {
			disk, err := os.ReadFile(st.Path)
			if err == nil {
				old = disk
			}
		}

		_, _ = fmt.Fprintln(out, previewDiff(st.Path, old, byPath[st.Path]))
	}
}

// unifiedDiff renders path's old→new as a minimal unified diff: common
// prefix/suffix lines trimmed, one hunk with up to three context lines each
// side. It is a preview, not a patch format: no multi-hunk splitting.
func previewDiff(path string, oldBytes, newBytes []byte) string {
	oldLines := splitDiffLines(oldBytes)
	newLines := splitDiffLines(newBytes)

	pre := 0
	for pre < len(oldLines) && pre < len(newLines) && oldLines[pre] == newLines[pre] {
		pre++
	}

	suf := 0
	for suf < len(oldLines)-pre && suf < len(newLines)-pre && oldLines[len(oldLines)-1-suf] == newLines[len(newLines)-1-suf] {
		suf++
	}

	oldMid := oldLines[pre : len(oldLines)-suf]
	newMid := newLines[pre : len(newLines)-suf]

	var b strings.Builder

	fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", filepath.ToSlash(path), filepath.ToSlash(path))
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", pre+1, len(oldMid), pre+1, len(newMid))

	for _, line := range contextLines(oldLines, pre) {
		b.WriteString(" " + line + "\n")
	}

	for _, line := range oldMid {
		b.WriteString("-" + line + "\n")
	}

	for _, line := range newMid {
		b.WriteString("+" + line + "\n")
	}

	for _, line := range contextLinesTail(newLines, len(newLines)-suf) {
		b.WriteString(" " + line + "\n")
	}

	return strings.TrimSuffix(b.String(), "\n")
}

// splitDiffLines splits content into lines without a trailing empty element.
func splitDiffLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}

	return strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
}

// contextLines returns up to three lines before at in lines.
func contextLines(lines []string, at int) []string {
	start := at - 3
	if start < 0 {
		start = 0
	}

	return lines[start:at]
}

// contextLinesTail returns up to three lines from at in lines.
func contextLinesTail(lines []string, at int) []string {
	if at < 0 {
		at = 0
	}

	end := at + 3
	if end > len(lines) {
		end = len(lines)
	}

	return lines[at:end]
}

// reportGenerateCheck backs `zever generate --check`: non-zero exit naming
// every missing/differing extra file, success otherwise. Missing service
// stubs count as drift (a fresh clone must create them); existing stubs
// never do (R1). Version-only differences in refreshed files are a note
// unless Strict.
func reportGenerateCheck(cfg GenerateFullConfig, out io.Writer, backendsSpec string, desired map[string][]byte, statuses []fileStatus, data serverData, scaffold map[string]bool) error {
	const tag = "zever generate"

	var drift []string

	var stampOnly []string

	for _, st := range statuses {
		if st.Status == fileUnchanged {
			continue
		}

		if st.Status == fileCreated {
			drift = append(drift, st.Path+" (missing)")

			continue
		}

		disk, err := os.ReadFile(st.Path)
		if err != nil {
			drift = append(drift, st.Path+" (missing)")

			continue
		}

		switch {
		case normalizeGeneratedBytes(st.Path, disk, desired[st.Path], scaffold[st.Path]):
			stampOnly = append(stampOnly, st.Path)
		default:
			drift = append(drift, st.Path+" (differs)")
		}
	}

	for _, st := range stubDrift(data) {
		drift = append(drift, st.Path+" (missing)")
	}

	if len(drift) == 0 && len(stampOnly) > 0 && cfg.Strict {
		drift = stampOnly
		for i, d := range drift {
			drift[i] = d + " (stamp version)"
		}
	}

	if len(drift) > 0 {
		return fmt.Errorf("%s: generated files differ in %d file(s) (%s); run 'zever generate' and commit the result",
			tag, len(drift), strings.Join(drift, ", "))
	}

	msg := success("✔ generated files are up to date ") + bold(fmt.Sprintf("%d file(s)", len(desired))) +
		dim(fmt.Sprintf(" (backends: %s)", backendsSpec))
	_, _ = fmt.Fprintln(out, msg)

	if len(stampOnly) > 0 {
		_, _ = fmt.Fprintln(out, dim(fmt.Sprintf("note: %d file(s) stamped by a different zever version than v%s; content and schema hash match. Re-run 'zever generate' to refresh the stamp, or pass --strict to fail on this.",
			len(stampOnly), buildVersion)))
	}

	return nil
}

// normalizeGeneratedBytes reports whether disk and fresh differ only in a
// stamp version token: compile outputs compare through
// normalizeStampVersion, refreshed scaffold files through the scaffold stamp
// lines.
func normalizeGeneratedBytes(path string, disk, fresh []byte, isScaffold bool) bool {
	if isScaffold {
		return string(stripScaffoldStamp(disk)) == string(stripScaffoldStamp(fresh))
	}

	return string(normalizeStampVersion(path, disk)) == string(normalizeStampVersion(path, fresh))
}

// stripScaffoldStamp removes the two leading scaffold stamp lines so a
// version-only stamp difference compares equal.
func stripScaffoldStamp(content []byte) []byte {
	lines := splitDiffLines(content)
	if len(lines) > 0 && strings.HasPrefix(lines[0], "// Code generated by zever generate server v") {
		lines = lines[1:]
	}

	if len(lines) > 0 && strings.HasPrefix(lines[0], "// schema-hash:") {
		lines = lines[1:]
	}

	return []byte(strings.Join(lines, "\n"))
}

// writeGenerateTree writes every changed desired file, creates missing
// service stubs (never touching existing ones), and prints the R2 summary
// diff of what changed.
func writeGenerateTree(tag string, out, stderr io.Writer, outDir, backendsSpec string, desired map[string][]byte, statuses []fileStatus, data serverData) error {
	var created, updated int

	for _, st := range statuses {
		if st.Status == fileUnchanged {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(st.Path), 0o750); err != nil {
			return fmt.Errorf("%s: mkdir %q: %w", tag, filepath.Dir(st.Path), err)
		}

		if err := os.WriteFile(st.Path, desired[st.Path], 0o644); err != nil { // generated source, not a secret
			return fmt.Errorf("%s: write %q: %w", tag, st.Path, err)
		}

		if st.Status == fileCreated {
			created++
		} else {
			updated++
		}
	}

	unchanged := len(statuses) - created - updated

	stubsWritten, err := writeServiceStubs(tag, data)
	if err != nil {
		return err
	}

	warnRemovedStubs(tag, data, stderr)

	summary := success("✔ generated ") + bold(fmt.Sprintf("%d file(s)", len(desired))) + dim(" → ") +
		cyan(outDir) + dim(fmt.Sprintf(" (%d created, %d updated, %d unchanged; backends: %s)", created, updated, unchanged, backendsSpec))
	_, _ = fmt.Fprintln(out, summary)

	if len(data.Modules) == 0 {
		_, _ = fmt.Fprintln(out, dim("no schema services: entrypoint and wiring rendered without modules"))
	} else {
		_, _ = fmt.Fprintln(out, dim(fmt.Sprintf("wired %d module(s) with %d stub(s) created", len(data.Modules), len(stubsWritten))))
	}

	return nil
}
