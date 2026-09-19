package main

import (
	"bytes"
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// ---- Doctor ----
// NOTE: DoctorConfig has no output-file field, so this screen renders doctor
// output in the result view only — no save-to-file option is invented.

type doctorVals struct{ configPath string }

// DoctorScreen wraps runDoctorWith: optional config path (empty = defaults).
type DoctorScreen struct {
	form  *huh.Form
	vals  *doctorVals
	stage inspectStage
	exec  tui.ExecModel
	theme tui.Theme
	keys  tui.Keymap
}

// NewDoctorScreen returns the doctor form.
func NewDoctorScreen() DoctorScreen {
	vals := &doctorVals{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Config path").Description("optional; empty verifies config.Default()").Placeholder("zever.yaml").Value(&vals.configPath),
	))
	return DoctorScreen{form: form, vals: vals, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
}

// CLI returns the exact equivalent CLI string. Pure.
func (m DoctorScreen) CLI() string { return buildDoctorCLI(m.vals.configPath) }

func (m DoctorScreen) buildExec() tui.ExecModel {
	cfg := strings.TrimSpace(m.vals.configPath)
	cli := buildDoctorCLI(cfg)
	return tui.NewExec("doctor", cli, makeDoctorExecFn(cfg))
}

// makeDoctorExecFn runs runDoctorWith into a buffer.
// Runs in a tea.Cmd: no I/O in Update.
func makeDoctorExecFn(configPath string) tui.ExecFunc {
	return func(ctx context.Context) (string, error) {
		if err := inspectCtxErr(ctx); err != nil {
			return "", err
		}
		var buf bytes.Buffer
		err := runDoctorWith(DoctorConfig{ConfigPath: configPath, Out: &buf})
		return buf.String(), err
	}
}

// Init implements tea.Model.
func (m DoctorScreen) Init() tea.Cmd { return m.form.Init() }

// Update implements tea.Model. No I/O.
func (m DoctorScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := updateInspectFormScreen(msg, &m.stage, &m.exec, m.keys, m.form, m.buildExec)
	m.form = form
	return m, cmd
}

// View implements tea.Model. Pure.
func (m DoctorScreen) View() tea.View {
	if m.stage == inspectStageExec {
		return m.exec.View()
	}
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("doctor"))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
	return tea.NewView(b.String())
}

// ---- Config ----
// NOTE: ConfigConfig has no output-file field, so this screen renders config
// output in the result view only — no save-to-file option is invented.

type configVals struct{ configPath string }

// ConfigScreen wraps runConfigShowWith: optional config path (empty =
// config.Default()).
type ConfigScreen struct {
	form  *huh.Form
	vals  *configVals
	stage inspectStage
	exec  tui.ExecModel
	theme tui.Theme
	keys  tui.Keymap
}

// NewConfigScreen returns the config form.
func NewConfigScreen() ConfigScreen {
	vals := &configVals{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Config path").Description("optional; empty verifies config.Default()").Placeholder("zever.yaml").Value(&vals.configPath),
	))
	return ConfigScreen{form: form, vals: vals, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
}

// CLI returns the exact equivalent CLI string. Pure.
func (m ConfigScreen) CLI() string { return buildConfigCLI(m.vals.configPath) }

func (m ConfigScreen) buildExec() tui.ExecModel {
	cfg := strings.TrimSpace(m.vals.configPath)
	cli := buildConfigCLI(cfg)
	return tui.NewExec("config show", cli, makeConfigExecFn(cfg))
}

// makeConfigExecFn runs runConfigShowWith into a buffer.
// Runs in a tea.Cmd: no I/O in Update.
func makeConfigExecFn(configPath string) tui.ExecFunc {
	return func(ctx context.Context) (string, error) {
		if err := inspectCtxErr(ctx); err != nil {
			return "", err
		}
		var buf bytes.Buffer
		err := runConfigShowWith(ConfigConfig{ConfigPath: configPath, Out: &buf})
		return buf.String(), err
	}
}

// Init implements tea.Model.
func (m ConfigScreen) Init() tea.Cmd { return m.form.Init() }

// Update implements tea.Model. No I/O.
func (m ConfigScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := updateInspectFormScreen(msg, &m.stage, &m.exec, m.keys, m.form, m.buildExec)
	m.form = form
	return m, cmd
}

// View implements tea.Model. Pure.
func (m ConfigScreen) View() tea.View {
	if m.stage == inspectStageExec {
		return m.exec.View()
	}
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("config show"))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
	return tea.NewView(b.String())
}

// ---- Routes ----

type routesVals struct{ files string }

// RoutesScreen wraps runRoutesWith: files input only.
type RoutesScreen struct {
	form  *huh.Form
	vals  *routesVals
	stage inspectStage
	exec  tui.ExecModel
	theme tui.Theme
	keys  tui.Keymap
}

// NewRoutesScreen returns the routes form (empty = auto-discover schema/).
func NewRoutesScreen() RoutesScreen {
	vals := &routesVals{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Schema files").Description("comma or space separated; empty = auto-discover schema/").Placeholder("schema/app.zen").Value(&vals.files),
	))
	return RoutesScreen{form: form, vals: vals, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
}

// CLI returns the exact equivalent CLI string. Pure.
func (m RoutesScreen) CLI() string { return buildRoutesCLI(splitFileList(m.vals.files)) }

func (m RoutesScreen) buildExec() tui.ExecModel {
	files := splitFileList(m.vals.files)
	cli := buildRoutesCLI(files)
	return tui.NewExec("routes", cli, makeRoutesExecFn(files))
}

// makeRoutesExecFn runs runRoutesWith into a buffer (auto-discovering files
// when empty). Runs in a tea.Cmd: no I/O in Update.
func makeRoutesExecFn(files []string) tui.ExecFunc {
	return func(ctx context.Context) (string, error) {
		if err := inspectCtxErr(ctx); err != nil {
			return "", err
		}
		if len(files) == 0 {
			files = discoverZenFiles()
		}
		var buf bytes.Buffer
		err := runRoutesWith(RoutesConfig{Files: files, Out: &buf})
		return buf.String(), err
	}
}

// Init implements tea.Model.
func (m RoutesScreen) Init() tea.Cmd { return m.form.Init() }

// Update implements tea.Model. No I/O.
func (m RoutesScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := updateInspectFormScreen(msg, &m.stage, &m.exec, m.keys, m.form, m.buildExec)
	m.form = form
	return m, cmd
}

// View implements tea.Model. Pure.
func (m RoutesScreen) View() tea.View {
	if m.stage == inspectStageExec {
		return m.exec.View()
	}
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("routes"))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
	return tea.NewView(b.String())
}

// ---- Explain ----

type explainVals struct {
	opPath string
	files  string
}

// ExplainScreen wraps runExplainWith: op path + files.
type ExplainScreen struct {
	form  *huh.Form
	vals  *explainVals
	stage inspectStage
	exec  tui.ExecModel
	theme tui.Theme
	keys  tui.Keymap
}

// NewExplainScreen returns the explain form (op path required).
func NewExplainScreen() ExplainScreen {
	vals := &explainVals{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Operation").Description("Service.Operation or Module.Service.Operation").Placeholder("UserService.GetUser").Validate(validateExplainOp).Value(&vals.opPath),
		huh.NewInput().Title("Schema files").Description("comma or space separated; empty = auto-discover schema/").Placeholder("schema/app.zen").Value(&vals.files),
	))
	return ExplainScreen{form: form, vals: vals, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
}

// CLI returns the exact equivalent CLI string. Pure.
func (m ExplainScreen) CLI() string {
	return buildExplainCLI(m.vals.opPath, splitFileList(m.vals.files))
}

func (m ExplainScreen) buildExec() tui.ExecModel {
	op := strings.TrimSpace(m.vals.opPath)
	files := splitFileList(m.vals.files)
	cli := buildExplainCLI(op, files)
	return tui.NewExec("explain", cli, makeExplainExecFn(op, files))
}

// makeExplainExecFn runs runExplainWith into a buffer (auto-discovering files
// when empty). Runs in a tea.Cmd: no I/O in Update.
func makeExplainExecFn(op string, files []string) tui.ExecFunc {
	return func(ctx context.Context) (string, error) {
		if err := inspectCtxErr(ctx); err != nil {
			return "", err
		}
		if len(files) == 0 {
			files = discoverZenFiles()
		}
		var buf bytes.Buffer
		err := runExplainWith(ExplainConfig{OpPath: op, Files: files, Out: &buf})
		return buf.String(), err
	}
}

// Init implements tea.Model.
func (m ExplainScreen) Init() tea.Cmd { return m.form.Init() }

// Update implements tea.Model. No I/O.
func (m ExplainScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := updateInspectFormScreen(msg, &m.stage, &m.exec, m.keys, m.form, m.buildExec)
	m.form = form
	return m, cmd
}

// View implements tea.Model. Pure.
func (m ExplainScreen) View() tea.View {
	if m.stage == inspectStageExec {
		return m.exec.View()
	}
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("explain"))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
	return tea.NewView(b.String())
}

// validateExplainOp rejects a blank operation path (mirrors errExplainUsage).
func validateExplainOp(s string) error {
	if strings.TrimSpace(s) == "" {
		return errExplainUsage
	}
	return nil
}

// ---- Boundaries ----

type boundariesVals struct{ files string }

// BoundariesScreen wraps runCheckBoundariesWith: files input, report-only.
type BoundariesScreen struct {
	form  *huh.Form
	vals  *boundariesVals
	stage inspectStage
	exec  tui.ExecModel
	theme tui.Theme
	keys  tui.Keymap
}

// NewBoundariesScreen returns the check-boundaries form.
func NewBoundariesScreen() BoundariesScreen {
	vals := &boundariesVals{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Schema files").Description("comma or space separated; empty = auto-discover schema/").Placeholder("schema/app.zen").Value(&vals.files),
	))
	return BoundariesScreen{form: form, vals: vals, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
}

// CLI returns the exact equivalent CLI string. Pure.
func (m BoundariesScreen) CLI() string { return buildBoundariesCLI(splitFileList(m.vals.files)) }

func (m BoundariesScreen) buildExec() tui.ExecModel {
	files := splitFileList(m.vals.files)
	cli := buildBoundariesCLI(files)
	return tui.NewExec("check-boundaries", cli, makeBoundariesExecFn(files))
}

// makeBoundariesExecFn runs runCheckBoundariesWith into a buffer
// (auto-discovering files when empty). Runs in a tea.Cmd: no I/O in Update.
func makeBoundariesExecFn(files []string) tui.ExecFunc {
	return func(ctx context.Context) (string, error) {
		if err := inspectCtxErr(ctx); err != nil {
			return "", err
		}
		if len(files) == 0 {
			files = discoverZenFiles()
		}
		var buf bytes.Buffer
		err := runCheckBoundariesWith(BoundariesConfig{Files: files, Out: &buf})
		return buf.String(), err
	}
}

// Init implements tea.Model.
func (m BoundariesScreen) Init() tea.Cmd { return m.form.Init() }

// Update implements tea.Model. No I/O.
func (m BoundariesScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := updateInspectFormScreen(msg, &m.stage, &m.exec, m.keys, m.form, m.buildExec)
	m.form = form
	return m, cmd
}

// View implements tea.Model. Pure.
func (m BoundariesScreen) View() tea.View {
	if m.stage == inspectStageExec {
		return m.exec.View()
	}
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("check-boundaries"))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
	return tea.NewView(b.String())
}

// ---- Graph ----
// NOTE: GraphConfig has no output-file field, so no save option is invented.
// Output renders in a pager-style scroll view via the kit log pane
// (tui.NewLog over bubbles/viewport).

type graphVals struct{ files string }

// GraphScreen wraps runGraphWith with a pager result view.
type GraphScreen struct {
	form   *huh.Form
	vals   *graphVals
	stage  inspectStage
	exec   tui.ExecModel
	log    tui.LogModel
	width  int
	height int
	theme  tui.Theme
	keys   tui.Keymap
}

// NewGraphScreen returns the graph form with an 80x24 pager.
func NewGraphScreen() GraphScreen {
	vals := &graphVals{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Schema files").Description("comma or space separated; empty = auto-discover schema/").Placeholder("schema/app.zen").Value(&vals.files),
	))
	return GraphScreen{form: form, vals: vals, log: tui.NewLog(80, 24), width: 80, height: 24, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
}

// CLI returns the exact equivalent CLI string. Pure.
func (m GraphScreen) CLI() string { return buildGraphCLI(splitFileList(m.vals.files)) }

func (m GraphScreen) buildExec() tui.ExecModel {
	files := splitFileList(m.vals.files)
	cli := buildGraphCLI(files)
	return tui.NewExec("graph", cli, makeGraphExecFn(files))
}

// makeGraphExecFn runs runGraphWith into a buffer (auto-discovering files
// when empty). Runs in a tea.Cmd: no I/O in Update.
func makeGraphExecFn(files []string) tui.ExecFunc {
	return func(ctx context.Context) (string, error) {
		if err := inspectCtxErr(ctx); err != nil {
			return "", err
		}
		if len(files) == 0 {
			files = discoverZenFiles()
		}
		var buf bytes.Buffer
		err := runGraphWith(GraphConfig{Files: files, Out: &buf})
		return buf.String(), err
	}
}

// Init implements tea.Model.
func (m GraphScreen) Init() tea.Cmd { return m.form.Init() }

// Update implements tea.Model. No I/O: pager content arrives via ExecDoneMsg.
func (m GraphScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.stage { //nolint:exhaustive // default forwards to the form, covering inspectStageForm and future stages
	case inspectStageExec:
		if km, ok := msg.(tea.KeyPressMsg); ok && m.keys.Back.Matches(km.String()) {
			if m.exec.State() != tui.ExecRunning {
				return m, inspectBackCmd()
			}
		}
		updated, cmd := m.exec.Update(msg)
		if em, ok := updated.(tui.ExecModel); ok {
			m.exec = em
			if em.State() == tui.ExecDone {
				m.log.Append(strings.Split(em.Output(), "\n")...)
				m.stage = inspectStageLog
				return m, cmd
			}
		}
		return m, cmd
	case inspectStageLog:
		if ws, ok := msg.(tea.WindowSizeMsg); ok {
			if ws.Width > 0 {
				m.width = ws.Width
			}
			if ws.Height > 0 {
				m.height = ws.Height
			}
			updated, cmd := m.log.Update(msg)
			if lm, ok := updated.(*tui.LogModel); ok {
				m.log = *lm
			}
			return m, cmd
		}
		if km, ok := msg.(tea.KeyPressMsg); ok && m.keys.Back.Matches(km.String()) {
			return m, inspectBackCmd()
		}
		updated, cmd := m.log.Update(msg)
		if lm, ok := updated.(*tui.LogModel); ok {
			m.log = *lm
		}
		if _, ok := msg.(tui.LogBackMsg); ok {
			return m, inspectBackCmd()
		}
		return m, cmd
	default:
		if ws, ok := msg.(tea.WindowSizeMsg); ok {
			if ws.Width > 0 {
				m.width = ws.Width
			}
			if ws.Height > 0 {
				m.height = ws.Height
			}
		}
		if km, ok := msg.(tea.KeyPressMsg); ok && m.keys.Back.Matches(km.String()) {
			return m, inspectBackCmd()
		}
		var fcmd tea.Cmd
		m.form, fcmd = forwardHuh(m.form, msg)
		if m.form.State == huh.StateCompleted {
			m.exec = m.buildExec()
			exec := m.exec
			m.stage = inspectStageExec
			return m, tea.Batch(fcmd, exec.Init(), exec.Start)
		}
		if m.form.State == huh.StateAborted {
			return m, inspectBackCmd()
		}
		return m, fcmd
	}
}

// View implements tea.Model. Pure.
func (m GraphScreen) View() tea.View {
	switch m.stage { //nolint:exhaustive // default forwards to the form, covering inspectStageForm and future stages
	case inspectStageExec:
		return m.exec.View()
	case inspectStageLog:
		var b strings.Builder
		b.WriteString(m.theme.Title.Render("graph"))
		b.WriteString("\n\n")
		b.WriteString(m.log.View().Content)
		b.WriteString("\n")
		b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.exec.CLI()))
		return tea.NewView(b.String())
	default:
		var b strings.Builder
		b.WriteString(m.theme.Title.Render("graph"))
		b.WriteString("\n\n")
		b.WriteString(m.form.View())
		b.WriteString("\n\n")
		b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
		b.WriteString("\n")
		b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
		return tea.NewView(b.String())
	}
}
