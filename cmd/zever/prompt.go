package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
)

// isInteractiveTerminal reports whether guided prompts may run: interactive
// mode was requested (via -i/--interactive or ZEVER_INTERACTIVE) and stdin is
// a terminal. The TTY probe reuses the isStdinTerminal seam from main.go so
// tests can stub it without a real terminal.
func isInteractiveTerminal() bool {
	if !interactiveMode && !envInteractive() {
		return false
	}

	return isStdinTerminal()
}

// requireInteractive returns nil when prompting is allowed, or an error
// explaining why not.
func requireInteractive() error {
	if isInteractiveTerminal() {
		return nil
	}

	if interactiveMode || envInteractive() {
		return errors.New("zever: --interactive requires a TTY (stdin not a terminal)")
	}

	return errors.New("missing required argument (use --interactive for guided prompts)")
}

// runForm executes a huh form against the terminal.
//
// Seam var so tests can drive forms headless (teatest virtual terminal)
// without a real TTY. Proof of behavior preservation: the default is exactly
// form.Run — same call, same arguments, same return — so every reachable
// input takes the identical path with or without this seam.
var runForm = func(f *huh.Form) error { return f.Run() }

// promptInput shows a huh Input and returns the trimmed value. The form
// renders to stderr (huh's default), never stdout, so prompted output stays
// out of piped data.
func promptInput(title, placeholder string, validate func(string) error) (string, error) {
	if err := requireInteractive(); err != nil {
		return "", err
	}

	var val string

	field := huh.NewInput().
		Title(title).
		Placeholder(placeholder).
		Value(&val)
	if validate != nil {
		field = field.Validate(validate)
	}

	form := huh.NewForm(huh.NewGroup(field))
	if err := runForm(form); err != nil {
		return "", fmt.Errorf("zever: prompt %q: %w", title, err)
	}

	return strings.TrimSpace(val), nil
}

// promptSelect shows a huh Select and returns the chosen value.
func promptSelect(title string, options []string) (string, error) {
	if err := requireInteractive(); err != nil {
		return "", err
	}

	if len(options) == 0 {
		return "", fmt.Errorf("no options for %q", title)
	}

	var val string

	opts := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		opts = append(opts, huh.NewOption(o, o))
	}

	field := huh.NewSelect[string]().
		Title(title).
		Options(opts...).
		Value(&val)

	form := huh.NewForm(huh.NewGroup(field))
	if err := runForm(form); err != nil {
		return "", fmt.Errorf("zever: prompt %q: %w", title, err)
	}

	return val, nil
}

// promptConfirm shows a huh Confirm and returns the choice.
func promptConfirm(title string, affirmative string) (bool, error) {
	if err := requireInteractive(); err != nil {
		return false, err
	}

	var val bool

	field := huh.NewConfirm().
		Title(title).
		Affirmative(affirmative).
		Negative("No").
		Value(&val)

	form := huh.NewForm(huh.NewGroup(field))
	if err := runForm(form); err != nil {
		return false, fmt.Errorf("zever: prompt %q: %w", title, err)
	}

	return val, nil
}

// hasInteractiveFlag checks if args contains -i/--interactive (for
// per-command FlagSet alias).
//
//nolint:unused
func hasInteractiveFlag(args []string) bool {
	for _, a := range args {
		if a == "-i" || a == "--interactive" {
			return true
		}
	}

	return envInteractive()
}

// promptMultiSelect shows a huh MultiSelect and returns chosen values.
func promptMultiSelect(title string, options []string) ([]string, error) {
	if err := requireInteractive(); err != nil {
		return nil, err
	}

	if len(options) == 0 {
		return nil, fmt.Errorf("no options for %q", title)
	}

	var vals []string

	opts := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		opts = append(opts, huh.NewOption(o, o))
	}

	field := huh.NewMultiSelect[string]().
		Title(title).
		Options(opts...).
		Value(&vals)

	form := huh.NewForm(huh.NewGroup(field))
	if err := runForm(form); err != nil {
		return nil, fmt.Errorf("zever: prompt %q: %w", title, err)
	}

	return vals, nil
}

// walkZenFiles recursively walks root and returns every .zen file path found,
// in WalkDir's (lexicographic, depth-first) order. This is the single shared
// implementation behind both discoverZenFiles (best-effort: swallows the
// error) and collectZenFiles (propagates it): flat (schema/*.zen), versioned
// (schema/v1/*.zen), or arbitrarily nested layouts all resolve the same way,
// since discovery walks directories rather than matching a glob pattern.
func walkZenFiles(root string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".zen") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// discoverZenFiles finds .zen files under the project's schema dir (or
// default). Best-effort: a missing or unreadable schema dir yields no files
// rather than an error, since callers already treat "no files found" as the
// signal to fall back or report errNoInputFiles.
func discoverZenFiles() []string {
	pc, err := loadProjectConfig()

	schemaDir := defaultSchemaDir
	if err == nil {
		schemaDir = pc.SchemaDir
	}

	files, _ := walkZenFiles(schemaDir)

	return files
}

// resolveInputFiles returns explicit if non-empty, else every .zen file
// auto-discovered under the project's schema dir (recursively, so flat,
// versioned, and per-entity-split layouts all resolve). errNoInputFiles is
// returned when neither yields anything.
func resolveInputFiles(explicit []string) ([]string, error) {
	if len(explicit) > 0 {
		return explicit, nil
	}

	discovered := discoverZenFiles()
	if len(discovered) == 0 {
		return nil, errNoInputFiles
	}

	return discovered, nil
}

// reportAutoDiscovery prints a hint noting that files were auto-discovered
// (rather than passed explicitly), when hints are enabled.
func reportAutoDiscovery(resolved []string) {
	if len(resolved) == 0 || !shouldShowHint() {
		return
	}

	schemaDir := defaultSchemaDir
	if pc, err := loadProjectConfig(); err == nil {
		schemaDir = pc.SchemaDir
	}

	_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf(
		"auto-discovered %d schema file(s) under %q", len(resolved), schemaDir,
	)))
}

// discoverModules returns module names discovered from .zen files.
// With dir-derived modules, the module is the immediate subdirectory under
// the schema dir containing .zen files.
func discoverModules() []string {
	pc, _ := loadProjectConfig()

	schemaDir := defaultSchemaDir
	if pc.SchemaDir != "" {
		schemaDir = pc.SchemaDir
	}

	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		return nil
	}

	var out []string

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		modDir := filepath.Join(schemaDir, e.Name())
		hasZen := false
		_ = filepath.WalkDir(modDir, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".zen") {
				hasZen = true
				return filepath.SkipDir
			}

			return nil
		})

		if hasZen {
			out = append(out, e.Name())
		}
	}

	return out
}
