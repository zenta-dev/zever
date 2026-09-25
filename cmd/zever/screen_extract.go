package main

// ExtractScreen is the TUI wizard for `zever extract`: module name plus
// optional output directory, module path and force flag — then preview,
// confirm, and an ExecModel run of the plan-then-write path. Schema
// compilation and go.mod reading happen inside the ExecFunc, never on the
// Update path.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// ExtractScreen extracts one schema module into a standalone service.
type ExtractScreen struct {
	flow       scaffoldFlow
	module     string
	outDir     string
	modulePath string
	force      bool
}

// NewExtractScreen builds the `zever extract` wizard with blank optionals
// (each falls back to the same default runExtract uses).
func NewExtractScreen() *ExtractScreen {
	m := &ExtractScreen{}

	m.flow = newScaffoldFlow("zever extract", m.buildForm())
	m.flow.onPrev = m.freezePreview
	m.flow.mkExec = m.buildExec

	return m
}

// buildForm constructs the wizard form bound to m's fields.
func (m *ExtractScreen) buildForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Module to extract").
				Validate(validateIdentField).
				Value(&m.module),
			huh.NewInput().
				Title("Output directory (optional)").
				Description("Defaults to ./<module>-service").
				Placeholder("./shop-service").
				Value(&m.outDir),
			huh.NewInput().
				Title("Module path (optional)").
				Description("Defaults to <this module>/<module>-service").
				Placeholder("example.com/shop-service").
				Value(&m.modulePath),
			huh.NewConfirm().
				Title("Overwrite files that already exist?").
				Affirmative("Force").
				Negative("No").
				Value(&m.force),
		),
	)
}

// buildConfig resolves the form values into the plan-then-write input,
// mirroring runExtract's defaults for blank optionals. Pure: no I/O.
func (m *ExtractScreen) buildConfig() ExtractConfig {
	return ExtractConfig{
		Module:     strings.TrimSpace(m.module),
		OutDir:     strings.TrimSpace(m.outDir),
		ModulePath: strings.TrimSpace(m.modulePath),
		Force:      m.force,
	}
}

// extractScreenCLI renders the exact equivalent CLI. Pure.
func extractScreenCLI(cfg ExtractConfig) string {
	var b strings.Builder

	b.WriteString("zever extract " + cliQuote(cfg.Module))
	if strings.TrimSpace(cfg.OutDir) != "" {
		b.WriteString(cliFlag("out", cfg.OutDir))
	}
	if strings.TrimSpace(cfg.ModulePath) != "" {
		b.WriteString(cliFlag("module", cfg.ModulePath))
	}
	b.WriteString(forceFlag(cfg.Force))

	return b.String()
}

// extractScreenPreview freezes the resolved plan into preview lines. Pure.
func extractScreenPreview(cfg ExtractConfig) []string {
	out := strings.TrimSpace(cfg.OutDir)
	if out == "" {
		out = "./" + cfg.Module + "-service (default)"
	}

	return []string{
		previewRow("module", cfg.Module),
		previewRow("out", out),
		previewRow("module path", nonEmptyOr(cfg.ModulePath, "<this module>/<module>-service")),
		previewRow("force", strconv.FormatBool(cfg.Force)),
	}
}

// freezePreview resolves the completed form into preview lines and CLI.
func (m *ExtractScreen) freezePreview() {
	cfg := m.buildConfig()
	m.flow.preview = extractScreenPreview(cfg)
	m.flow.cli = extractScreenCLI(cfg)
}

// buildExec wires the frozen config to the plan-then-write path.
func (m *ExtractScreen) buildExec() tui.ExecModel {
	cfg := m.buildConfig()
	cli := extractScreenCLI(cfg)

	return tui.NewExec("zever extract "+cfg.Module, cli, func(context.Context) (string, error) {
		return runExtractScreen(cfg) //nolint:contextcheck // CLI entrypoint helpers take no ctx by convention; threading ctx through the CLI layer is out of scope
	})
}

// runExtractScreen compiles the workspace schema, plans the extraction and
// writes it. It mirrors runExtract minus flag parsing. Note: writeExtraction
// prints its own summary to stdout; the returned string feeds the result
// view, so both carry the outcome.
func runExtractScreen(cfg ExtractConfig) (string, error) {
	const tag = "zever extract"

	project, err := loadProjectConfig()
	if err != nil {
		return "", err
	}

	gomod, err := readGoMod()
	if err != nil {
		return "", err
	}

	schema, err := compileSchemaDir(tag, project.SchemaDir)
	if err != nil {
		return "", err
	}

	plan, err := planExtraction(tag, cfg, schema, gomod)
	if err != nil {
		return "", err
	}

	if err := writeExtraction(tag, plan, gomod); err != nil {
		return "", err
	}

	return fmt.Sprintf("extracted module %q into %s", plan.Module.Name, plan.OutDir), nil
}

// Init implements tea.Model.
func (m *ExtractScreen) Init() tea.Cmd { return m.flow.initFlow() }

// Update implements tea.Model. No I/O; work starts as returned Cmds.
func (m *ExtractScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, m.flow.update(msg)
}

// View implements tea.Model. Pure.
func (m *ExtractScreen) View() tea.View { return tea.NewView(m.flow.view()) }
