package main

// NewScreen is the TUI wizard for `zever new`: app name, optional module
// path and output directory, db/cache/queue adapter picks, backend and
// battery multiselects, and a force confirm — then preview, confirm, and an
// ExecModel run of writeNewProject. The flag-driven runNew path has no
// adapter/battery flags, so those picks are TUI-only and never appear in the
// equivalent CLI line.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// NewScreen scaffolds a new service through a huh form. Bound values live on
// the struct so the form writes them via pointers; tests set them directly.
type NewScreen struct {
	flow      scaffoldFlow
	name      string
	module    string
	outDir    string
	db        string
	cache     string
	queue     string
	backends  []string
	batteries []string
	force     bool
}

// NewScaffoldScreen builds the `zever new` wizard with guided defaults: the
// historical quickstart backend sample preselected, sqlite/memory/memory
// adapters, core-only batteries, no force.
func NewScaffoldScreen() *NewScreen {
	m := &NewScreen{
		db:       "sqlite",
		cache:    "memory",
		queue:    "memory",
		backends: append([]string(nil), defaultNewBackends...),
	}

	m.flow = newScaffoldFlow("zever new", m.buildForm())
	m.flow.onPrev = m.freezePreview
	m.flow.mkExec = m.buildExec

	return m
}

// buildForm constructs the wizard form bound to m's fields.
func (m *NewScreen) buildForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("App name").
				Description("Directory, module default, README title").
				Placeholder("myapp").
				Validate(validateAppNameField).
				Value(&m.name),
			huh.NewInput().
				Title("Module path (optional)").
				Description("Go module path; defaults to the app name").
				Placeholder("example.com/myapp").
				Validate(validateOptionalIdentField).
				Value(&m.module),
			huh.NewInput().
				Title("Output directory (optional)").
				Description("Defaults to ./<name>").
				Placeholder("./myapp").
				Value(&m.outDir),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Database adapter").
				Options(huh.NewOptions("sqlite", "postgres", "mysql")...).
				Value(&m.db),
			huh.NewSelect[string]().
				Title("Cache adapter").
				Options(huh.NewOptions("memory", "redis")...).
				Value(&m.cache),
			huh.NewSelect[string]().
				Title("Queue adapter").
				Options(huh.NewOptions("memory", "redis")...).
				Value(&m.queue),
			huh.NewMultiSelect[string]().
				Title("Backends for the README quickstart").
				Options(huh.NewOptions(backendNamesSlice()...)...).
				Value(&m.backends),
			huh.NewMultiSelect[string]().
				Title("Extra batteries (core is always included)").
				Options(huh.NewOptions(nonCoreBatteryNames()...)...).
				Value(&m.batteries),
			huh.NewConfirm().
				Title("Scaffold into a non-empty directory if needed?").
				Affirmative("Force").
				Negative("No").
				Value(&m.force),
		),
	)
}

// buildConfig resolves the form values into the single writeNewProject
// input. Only non-default adapter picks become overrides, keeping the flag
// path's Batteries-minimal semantics; empty module/outDir fall back to the
// same defaults runNew uses. Pure: no I/O.
func (m *NewScreen) buildConfig() NewConfig {
	name := strings.TrimSpace(m.name)
	outDir := strings.TrimSpace(m.outDir)
	if outDir == "" {
		outDir = "./" + name
	}

	modulePath := strings.TrimSpace(m.module)
	if modulePath == "" {
		modulePath = name
	}

	dbAdapter, cacheAdapter, queueAdapter := "", "", ""
	if m.db != "" && m.db != "sqlite" {
		dbAdapter = m.db
	}
	if m.cache != "" && m.cache != "memory" {
		cacheAdapter = m.cache
	}
	if m.queue != "" && m.queue != "memory" {
		queueAdapter = m.queue
	}

	return NewConfig{
		Name:         name,
		OutDir:       outDir,
		ModulePath:   modulePath,
		Force:        m.force,
		DBAdapter:    dbAdapter,
		CacheAdapter: cacheAdapter,
		QueueAdapter: queueAdapter,
		Batteries:    batteriesFor(m.batteries, cacheAdapter),
		Backends:     append([]string(nil), m.backends...),
	}
}

// newScreenCLI renders the exact equivalent CLI. Adapter and battery picks
// have no `zever new` flags, so they are preview-only by design.
func newScreenCLI(cfg NewConfig) string {
	var b strings.Builder

	b.WriteString("zever new " + cliQuote(cfg.Name))

	out := strings.TrimSpace(cfg.OutDir)
	if out != "" && out != "./"+cfg.Name {
		b.WriteString(cliFlag("dir", out))
	}
	if cfg.ModulePath != "" && cfg.ModulePath != cfg.Name {
		b.WriteString(cliFlag("module", cfg.ModulePath))
	}
	if cfg.Force {
		b.WriteString(" --force")
	}

	return b.String()
}

// newScreenPreview freezes the resolved plan into preview lines. No secrets
// pass through this screen, so every value is safe to show.
func newScreenPreview(cfg NewConfig) []string {
	adapters := fmt.Sprintf("db=%s cache=%s queue=%s",
		nonEmptyOr(cfg.DBAdapter, "sqlite"),
		nonEmptyOr(cfg.CacheAdapter, "memory"),
		nonEmptyOr(cfg.QueueAdapter, "memory"))

	return []string{
		previewRow("app", cfg.Name),
		previewRow("dir", cfg.OutDir),
		previewRow("module", cfg.ModulePath),
		previewRow("adapters", adapters),
		previewRow("batteries", strings.Join(cfg.Batteries, ", ")),
		previewRow("backends", quickstartBackends(cfg)),
		previewRow("force", strconv.FormatBool(cfg.Force)),
	}
}

// nonEmptyOr returns s unless it is blank, when it returns fallback.
func nonEmptyOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}

	return s
}

// freezePreview resolves the completed form into the frozen preview lines
// and CLI string shown in stagePreview. Pure: no I/O.
func (m *NewScreen) freezePreview() {
	cfg := m.buildConfig()
	m.flow.preview = newScreenPreview(cfg)
	m.flow.cli = newScreenCLI(cfg)
}

// buildExec wires the frozen config to the single writer. Resolution that
// touches the filesystem (framework checkout, target dir) runs inside the
// ExecFunc, off the Update path.
func (m *NewScreen) buildExec() tui.ExecModel {
	cfg := m.buildConfig()
	cli := newScreenCLI(cfg)

	return tui.NewExec("zever new "+cfg.Name, cli, func(context.Context) (string, error) {
		return runNewScreen(cfg)
	})
}

// runNewScreen resolves the framework, guards the target directory, and
// writes the project through THE single writer. It mirrors runNew's plan
// path minus flag parsing.
func runNewScreen(cfg NewConfig) (string, error) {
	const tag = "zever new"

	if err := resolveFramework(tag, &cfg, "", ""); err != nil {
		return "", err
	}
	if err := ensureTargetDir(tag, cfg.OutDir, cfg.Force); err != nil {
		return "", err
	}

	written, err := writeNewProject(tag, cfg)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("scaffolded %q into %s (%d files)", cfg.Name, cfg.OutDir, len(written)), nil
}

// Init implements tea.Model.
func (m *NewScreen) Init() tea.Cmd { return m.flow.initFlow() }

// Update implements tea.Model. No I/O; work starts as returned Cmds.
func (m *NewScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, m.flow.update(msg)
}

// View implements tea.Model. Pure.
func (m *NewScreen) View() tea.View { return tea.NewView(m.flow.view()) }
