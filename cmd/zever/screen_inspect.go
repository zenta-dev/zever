// Inspect TUI screens: form → exec wrappers over the runXWith cores.
//
// Each screen collects its core Config via an embedded huh form (no I/O in
// Update), then runs the core inside a tui.ExecModel Cmd and shows the exact
// equivalent CLI string. esc never writes: in form phase it emits tui.BackMsg,
// in exec phase running work cancels via ExecModel and finished phases go back.
package main

import (
	"bytes"
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// inspectStage is the form → exec lifecycle shared by every Inspect screen.
type inspectStage int

const (
	inspectStageForm inspectStage = iota
	inspectStageExec
	inspectStageLog // graph only: pager over exec output
)

// inspectDefaultOutDir mirrors runCompile's --out default.
const inspectDefaultOutDir = "./generated"

// splitFileList parses multi-file text input: comma, space, newline and
// semicolon separated. Empty entries are dropped. Pure.
func splitFileList(s string) []string {
	clean := strings.ReplaceAll(s, ",", " ")
	clean = strings.ReplaceAll(clean, ";", " ")
	fields := strings.Fields(clean)
	out := fields[:0]
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// joinCLI joins already-split args with single spaces. Pure.
func joinCLI(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " ")
}

// filesOrEmpty renders a file list for CLI preview. Pure.
func filesOrEmpty(files []string) string {
	return strings.Join(files, " ")
}

// inspectBackCmd emits tui.BackMsg: the esc-back contract. Pure.
func inspectBackCmd() tea.Cmd {
	return func() tea.Msg { return tui.BackMsg{} }
}

// inspectCtxErr maps a canceled context to an error the ExecModel normalizes
// to ExecCanceled instead of ExecFailed. Pure.
func inspectCtxErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// forwardHuh forwards msg to form. huh v2 Update always returns *huh.Form
// for a *huh.Form receiver (huh.Model contract), so the assertion holds.
func forwardHuh(form *huh.Form, msg tea.Msg) (*huh.Form, tea.Cmd) {
	updated, cmd := form.Update(msg)
	return updated.(*huh.Form), cmd //nolint:forcetypeassert // huh contract: *Form.Update returns *Form
}

// updateInspectFormScreen advances one form→exec inspect screen through the
// lifecycle every Check/Breaking/Fmt/Doctor/Routes/Explain/Boundaries screen
// shares: exec-phase keys go to the ExecModel (esc backs out once finished),
// form-phase esc backs out, and a completed form builds the exec and starts
// it. It mutates stage and exec in place, returns the (possibly new) form
// plus the command. Identical to the inlined Update bodies it replaces.
func updateInspectFormScreen(msg tea.Msg, stage *inspectStage, exec *tui.ExecModel, keys tui.Keymap, form *huh.Form, buildExec func() tui.ExecModel) (*huh.Form, tea.Cmd) {
	if *stage == inspectStageExec {
		if km, ok := msg.(tea.KeyPressMsg); ok && keys.Back.Matches(km.String()) {
			if exec.State() != tui.ExecRunning {
				return form, inspectBackCmd()
			}
		}
		updated, cmd := exec.Update(msg)
		if em, ok := updated.(tui.ExecModel); ok {
			*exec = em
		}
		return form, cmd
	}
	if km, ok := msg.(tea.KeyPressMsg); ok && keys.Back.Matches(km.String()) {
		return form, inspectBackCmd()
	}
	nform, fcmd := forwardHuh(form, msg)
	if nform.State == huh.StateCompleted {
		*exec = buildExec()
		started := *exec
		*stage = inspectStageExec
		return nform, tea.Batch(fcmd, started.Init(), started.Start)
	}
	if nform.State == huh.StateAborted {
		return nform, inspectBackCmd()
	}
	return nform, fcmd
}

// ---- CLI builders (pure; mirror runX flag parsing exactly) ----

func buildCompileCLI(files []string, backends, outDir string) string {
	s := "zever compile"
	if strings.TrimSpace(backends) != "" {
		s += " --backend=" + strings.TrimSpace(backends)
	}
	if strings.TrimSpace(outDir) != "" {
		s += " --out=" + strings.TrimSpace(outDir)
	}
	if f := filesOrEmpty(files); f != "" {
		s += " " + f
	}
	return s
}

func buildCheckCLI(files []string) string {
	return joinCLI("zever check", filesOrEmpty(files))
}

func buildBreakingCLI(oldFiles, newFiles []string) string {
	s := "zever breaking"
	if f := filesOrEmpty(oldFiles); f != "" {
		s += " " + f
	}
	s += " --"
	if f := filesOrEmpty(newFiles); f != "" {
		s += " " + f
	}
	return s
}

func buildFmtCLI(files []string, write bool) string {
	s := "zever fmt"
	if write {
		s += " --write"
	}
	if f := filesOrEmpty(files); f != "" {
		s += " " + f
	}
	return s
}

func buildDoctorCLI(configPath string) string {
	if strings.TrimSpace(configPath) == "" {
		return "zever doctor"
	}
	return "zever doctor --config " + strings.TrimSpace(configPath)
}

func buildRoutesCLI(files []string) string {
	return joinCLI("zever routes", filesOrEmpty(files))
}

func buildExplainCLI(opPath string, files []string) string {
	return joinCLI("zever explain", strings.TrimSpace(opPath), filesOrEmpty(files))
}

func buildBoundariesCLI(files []string) string {
	return joinCLI("zever check-boundaries", filesOrEmpty(files))
}

func buildGraphCLI(files []string) string {
	return joinCLI("zever graph", filesOrEmpty(files))
}

// RegisterInspectScreens exposes every Inspect screen for the dashboard.
// Group is tui.GroupInspect; Screen names the constructor (dashboard contract).
func RegisterInspectScreens() []tui.Entry {
	return []tui.Entry{
		{Group: tui.GroupInspect, Name: "compile", Desc: "compile schemas through backends", CLI: "zever compile", Screen: "NewCompileScreen"},
		{Group: tui.GroupInspect, Name: "check", Desc: "validate schemas, no output written", CLI: "zever check", Screen: "NewCheckScreen"},
		{Group: tui.GroupInspect, Name: "breaking", Desc: "report API-breaking changes", CLI: "zever breaking", Screen: "NewBreakingScreen"},
		{Group: tui.GroupInspect, Name: "fmt", Desc: "format .zen schemas", CLI: "zever fmt", Screen: "NewFmtScreen"},
		{Group: tui.GroupInspect, Name: "doctor", Desc: "verify batteries", CLI: "zever doctor", Screen: "NewDoctorScreen"},
		{Group: tui.GroupInspect, Name: "routes", Desc: "list HTTP routes", CLI: "zever routes", Screen: "NewRoutesScreen"},
		{Group: tui.GroupInspect, Name: "explain", Desc: "print an operation summary", CLI: "zever explain", Screen: "NewExplainScreen"},
		{Group: tui.GroupInspect, Name: "check-boundaries", Desc: "report cross-module violations", CLI: "zever check-boundaries", Screen: "NewBoundariesScreen"},
		{Group: tui.GroupInspect, Name: "graph", Desc: "print Mermaid diagrams", CLI: "zever graph", Screen: "NewGraphScreen"},
	}
}

// ---- Compile ----

type compileVals struct {
	files    string
	backends []string
	outDir   string
}

// CompileScreen wraps runCompileWith: files input + backend MultiSelect + out dir.
type CompileScreen struct {
	form  *huh.Form
	vals  *compileVals
	stage inspectStage
	exec  tui.ExecModel
	theme tui.Theme
	keys  tui.Keymap
}

// NewCompileScreen returns a compile form with all backends pre-selected and
// out defaulting to ./generated (mirrors runCompile flags).
func NewCompileScreen() CompileScreen {
	vals := &compileVals{outDir: inspectDefaultOutDir}
	vals.backends = append(vals.backends, backendNamesSlice()...)
	opts := make([]huh.Option[string], 0, len(vals.backends))
	for _, b := range backendNamesSlice() {
		opts = append(opts, huh.NewOption(b, b).Selected(true))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Schema files").Description("comma or space separated; empty = auto-discover schema/").Placeholder("schema/app.zen").Value(&vals.files),
		huh.NewMultiSelect[string]().Title("Backends").Description("space toggles; joined with \",\"").Options(opts...).Value(&vals.backends),
		huh.NewInput().Title("Out dir").Description("matches --out").Placeholder(inspectDefaultOutDir).Value(&vals.outDir),
	))
	return CompileScreen{form: form, vals: vals, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
}

// CLI returns the exact equivalent CLI string for current inputs. Pure.
func (m CompileScreen) CLI() string {
	backends := strings.Join(m.vals.backends, ",")
	return buildCompileCLI(splitFileList(m.vals.files), backends, strings.TrimSpace(m.vals.outDir))
}

func (m CompileScreen) selectedBackends() []string {
	return m.vals.backends
}

func (m CompileScreen) buildExec() tui.ExecModel {
	files := splitFileList(m.vals.files)
	backends := strings.Join(m.selectedBackends(), ",")
	outDir := strings.TrimSpace(m.vals.outDir)
	if outDir == "" {
		outDir = inspectDefaultOutDir
	}
	cli := buildCompileCLI(files, backends, outDir)
	return tui.NewExec("compile", cli, makeCompileExecFn(files, backends, outDir))
}

// makeCompileExecFn runs runCompileWith into a buffer (auto-discovering files
// when empty). Runs in a tea.Cmd: no I/O in Update.
func makeCompileExecFn(files []string, backends, outDir string) tui.ExecFunc {
	return func(ctx context.Context) (string, error) {
		if err := inspectCtxErr(ctx); err != nil {
			return "", err
		}
		if len(files) == 0 {
			files = discoverZenFiles()
		}
		var buf bytes.Buffer
		err := runCompileWith(CompileConfig{Files: files, Backends: backends, OutDir: outDir, Out: &buf})
		return buf.String(), err
	}
}

// Init implements tea.Model.
func (m CompileScreen) Init() tea.Cmd { return m.form.Init() }

// Update implements tea.Model. No I/O: cores run in ExecModel Cmds.
func (m CompileScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.stage {
	case inspectStageExec:
		if km, ok := msg.(tea.KeyPressMsg); ok && m.keys.Back.Matches(km.String()) {
			if m.exec.State() != tui.ExecRunning {
				return m, inspectBackCmd()
			}
		}
		updated, cmd := m.exec.Update(msg)
		if em, ok := updated.(tui.ExecModel); ok {
			m.exec = em
		}
		return m, cmd
	default:
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
func (m CompileScreen) View() tea.View {
	if m.stage == inspectStageExec {
		return m.exec.View()
	}
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("compile"))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
	return tea.NewView(b.String())
}

// ---- Check ----

type checkVals struct{ files string }

// CheckScreen wraps runCheckWith: files input only, writes nothing.
type CheckScreen struct {
	form  *huh.Form
	vals  *checkVals
	stage inspectStage
	exec  tui.ExecModel
	theme tui.Theme
	keys  tui.Keymap
}

// NewCheckScreen returns a check form (empty = auto-discover schema/).
func NewCheckScreen() CheckScreen {
	vals := &checkVals{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Schema files").Description("comma or space separated; empty = auto-discover schema/").Placeholder("schema/app.zen").Value(&vals.files),
	))
	return CheckScreen{form: form, vals: vals, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
}

// CLI returns the exact equivalent CLI string. Pure.
func (m CheckScreen) CLI() string { return buildCheckCLI(splitFileList(m.vals.files)) }

func (m CheckScreen) buildExec() tui.ExecModel {
	files := splitFileList(m.vals.files)
	cli := buildCheckCLI(files)
	return tui.NewExec("check", cli, makeCheckExecFn(files))
}

// makeCheckExecFn runs runCheckWith into a buffer (auto-discovering files
// when empty). Runs in a tea.Cmd: no I/O in Update.
func makeCheckExecFn(files []string) tui.ExecFunc {
	return func(ctx context.Context) (string, error) {
		if err := inspectCtxErr(ctx); err != nil {
			return "", err
		}
		if len(files) == 0 {
			files = discoverZenFiles()
		}
		var buf bytes.Buffer
		err := runCheckWith(CheckConfig{Files: files, Out: &buf})
		return buf.String(), err
	}
}

// Init implements tea.Model.
func (m CheckScreen) Init() tea.Cmd { return m.form.Init() }

// Update implements tea.Model. No I/O.
func (m CheckScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := updateInspectFormScreen(msg, &m.stage, &m.exec, m.keys, m.form, m.buildExec)
	m.form = form
	return m, cmd
}

// View implements tea.Model. Pure.
func (m CheckScreen) View() tea.View {
	if m.stage == inspectStageExec {
		return m.exec.View()
	}
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("check"))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
	return tea.NewView(b.String())
}

// ---- Breaking ----

type breakingVals struct {
	oldFiles string
	newFiles string
}

// BreakingScreen wraps runBreakingWith: old files + new files, CI-gate semantics.
type BreakingScreen struct {
	form  *huh.Form
	vals  *breakingVals
	stage inspectStage
	exec  tui.ExecModel
	theme tui.Theme
	keys  tui.Keymap
}

// NewBreakingScreen returns the old/new file form (mirrors "<old...> -- <new...>").
func NewBreakingScreen() BreakingScreen {
	vals := &breakingVals{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Old schema files").Description("comma or space separated").Placeholder("old/app.zen").Value(&vals.oldFiles),
		huh.NewInput().Title("New schema files").Description("comma or space separated").Placeholder("schema/app.zen").Value(&vals.newFiles),
	))
	return BreakingScreen{form: form, vals: vals, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
}

// CLI returns the exact equivalent CLI string. Pure.
func (m BreakingScreen) CLI() string {
	return buildBreakingCLI(splitFileList(m.vals.oldFiles), splitFileList(m.vals.newFiles))
}

func (m BreakingScreen) buildExec() tui.ExecModel {
	oldFiles := splitFileList(m.vals.oldFiles)
	newFiles := splitFileList(m.vals.newFiles)
	cli := buildBreakingCLI(oldFiles, newFiles)
	return tui.NewExec("breaking", cli, makeBreakingExecFn(oldFiles, newFiles))
}

// makeBreakingExecFn runs runBreakingWith into a buffer.
// Runs in a tea.Cmd: no I/O in Update.
func makeBreakingExecFn(oldFiles, newFiles []string) tui.ExecFunc {
	return func(ctx context.Context) (string, error) {
		if err := inspectCtxErr(ctx); err != nil {
			return "", err
		}
		var buf bytes.Buffer
		err := runBreakingWith(BreakingConfig{OldFiles: oldFiles, NewFiles: newFiles, Out: &buf})
		return buf.String(), err
	}
}

// Init implements tea.Model.
func (m BreakingScreen) Init() tea.Cmd { return m.form.Init() }

// Update implements tea.Model. No I/O.
func (m BreakingScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := updateInspectFormScreen(msg, &m.stage, &m.exec, m.keys, m.form, m.buildExec)
	m.form = form
	return m, cmd
}

// View implements tea.Model. Pure.
func (m BreakingScreen) View() tea.View {
	if m.stage == inspectStageExec {
		return m.exec.View()
	}
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("breaking"))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
	return tea.NewView(b.String())
}

// ---- Fmt ----

type fmtVals struct {
	files string
	write bool
}

// FmtScreen wraps runFmtWith: files input + Write confirm (-w/--write).
type FmtScreen struct {
	form  *huh.Form
	vals  *fmtVals
	stage inspectStage
	exec  tui.ExecModel
	theme tui.Theme
	keys  tui.Keymap
}

// NewFmtScreen returns the fmt form (Write=false default = list-only like gofmt -l).
func NewFmtScreen() FmtScreen {
	vals := &fmtVals{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Schema files").Description("comma or space separated; empty = auto-discover schema/").Placeholder("schema/app.zen").Value(&vals.files),
		huh.NewConfirm().Title("Rewrite files in place?").Description("yes = --write (gofmt -w); no = list only (gofmt -l)").Affirmative("Write").Negative("List only").Value(&vals.write),
	))
	return FmtScreen{form: form, vals: vals, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
}

// CLI returns the exact equivalent CLI string. Pure.
func (m FmtScreen) CLI() string { return buildFmtCLI(splitFileList(m.vals.files), m.vals.write) }

func (m FmtScreen) buildExec() tui.ExecModel {
	files := splitFileList(m.vals.files)
	write := m.vals.write
	cli := buildFmtCLI(files, write)
	return tui.NewExec("fmt", cli, makeFmtExecFn(files, write))
}

// makeFmtExecFn runs runFmtWith into a buffer (auto-discovering files when
// empty). Runs in a tea.Cmd: no I/O in Update.
func makeFmtExecFn(files []string, write bool) tui.ExecFunc {
	return func(ctx context.Context) (string, error) {
		if err := inspectCtxErr(ctx); err != nil {
			return "", err
		}
		if len(files) == 0 {
			files = discoverZenFiles()
		}
		var buf bytes.Buffer
		err := runFmtWith(FmtConfig{Files: files, Write: write, Out: &buf})
		return buf.String(), err
	}
}

// Init implements tea.Model.
func (m FmtScreen) Init() tea.Cmd { return m.form.Init() }

// Update implements tea.Model. No I/O.
func (m FmtScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := updateInspectFormScreen(msg, &m.stage, &m.exec, m.keys, m.form, m.buildExec)
	m.form = form
	return m, cmd
}

// View implements tea.Model. Pure.
func (m FmtScreen) View() tea.View {
	if m.stage == inspectStageExec {
		return m.exec.View()
	}
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("fmt"))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
	return tea.NewView(b.String())
}
