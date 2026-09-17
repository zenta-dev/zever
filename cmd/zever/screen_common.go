package main

// Scaffold TUI screens: shared form → preview → exec plumbing.
//
// Each screen in this group (new, generate, extract) is a huh form that
// builds a core config, a read-only preview with the equivalent CLI line,
// and a tui.ExecModel run. This file owns the stage machine so the three
// screens stay identical in behavior: esc backs out without writing
// anything, enter confirms from preview, and all I/O runs inside ExecFunc
// commands, never on the Update path.
//
// Pure helpers (validation, field-list parsing, CLI quoting) live here so
// tests drive them without a terminal.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// scaffoldStage is the lifecycle phase of a scaffold screen.
type scaffoldStage int

const (
	// stageForm collects input through the embedded huh form.
	stageForm scaffoldStage = iota
	// stagePreview shows the resolved plan plus the equivalent CLI line.
	stagePreview
	// stageExec runs the core and shows its result view.
	stageExec
)

// scaffoldFlow is the reusable form → preview → exec machine. Screens embed
// it by value and set form, onPreview and buildExec before first Update.
// title names the screen, preview holds the frozen plan lines shown in
// stagePreview, and cli is the exact equivalent CLI invocation.
type scaffoldFlow struct {
	title   string
	theme   tui.Theme
	keys    tui.Keymap
	width   int
	height  int
	stage   scaffoldStage
	form    *huh.Form
	preview []string
	cli     string
	exec    tui.ExecModel
	hasExec bool
	onPrev  func()
	mkExec  func() tui.ExecModel
}

// newScaffoldFlow returns a flow with the shared theme and keymap.
func newScaffoldFlow(title string, form *huh.Form) scaffoldFlow {
	return scaffoldFlow{
		title:  title,
		theme:  tui.NewTheme(),
		keys:   tui.DefaultKeymap(),
		width:  80,
		height: 24,
		form:   form,
	}
}

// Init implements tea.Model. It arms the embedded form; the exec command is
// scheduled on preview confirm, never here, so construction does no I/O.
func (f *scaffoldFlow) initFlow() tea.Cmd {
	if f.form == nil {
		return nil
	}

	return f.form.Init()
}

// setSize records the window and forwards it to the embedded form so huh
// lays out at the real width. Pure: no I/O.
func (f *scaffoldFlow) setSize(w, h int) tea.Cmd {
	if w > 0 {
		f.width = w
	}
	if h > 0 {
		f.height = h
	}
	if f.form == nil {
		return nil
	}

	return f.forwardToForm(tea.WindowSizeMsg{Width: f.width, Height: f.height})
}

// forwardToForm routes msg through the huh form and keeps the *huh.Form
// pointer fresh (huh v2 Update returns the compat Model interface).
func (f *scaffoldFlow) forwardToForm(msg tea.Msg) tea.Cmd {
	updated, cmd := f.form.Update(msg)
	if nf, ok := updated.(*huh.Form); ok {
		f.form = nf
	}

	return cmd
}

// update routes msg by stage. esc before exec emits BackMsg (nothing is
// written); enter on preview builds the exec and starts it. All other keys
// go to the form or the exec model. No I/O: work starts as returned Cmds.
func (f *scaffoldFlow) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return f.setSize(msg.Width, msg.Height)
	case tea.KeyPressMsg:
		key := msg.String()
		switch f.stage {
		case stageForm:
			if f.keys.Back.Matches(key) {
				return func() tea.Msg { return tui.BackMsg{} }
			}

			cmd := f.forwardToForm(msg)
			if f.form.State == huh.StateCompleted {
				f.stage = stagePreview
				if f.onPrev != nil {
					f.onPrev()
				}
			}

			return cmd
		case stagePreview:
			switch {
			case f.keys.Back.Matches(key):
				return func() tea.Msg { return tui.BackMsg{} }
			case f.keys.Select.Matches(key):
				f.exec = f.mkExec()
				f.hasExec = true
				f.stage = stageExec
				return tea.Batch(f.exec.Init(), f.exec.Start)
			default:
				return nil
			}
		case stageExec:
			if f.keys.Back.Matches(key) && f.exec.State() != tui.ExecRunning {
				return func() tea.Msg { return tui.BackMsg{} }
			}

			updated, cmd := f.exec.Update(msg)
			if em, ok := updated.(tui.ExecModel); ok {
				f.exec = em
			}

			return cmd
		}
	case tui.ExecDoneMsg, tui.ExecErrMsg:
		if f.hasExec {
			updated, cmd := f.exec.Update(msg)
			if em, ok := updated.(tui.ExecModel); ok {
				f.exec = em
			}

			return cmd
		}
	}

	if f.stage == stageForm && f.form != nil {
		return f.forwardToForm(msg)
	}
	if f.stage == stageExec && f.hasExec {
		updated, cmd := f.exec.Update(msg)
		if em, ok := updated.(tui.ExecModel); ok {
			f.exec = em
		}

		return cmd
	}

	return nil
}

// view renders the current stage. Pure.
func (f *scaffoldFlow) view() string {
	var b strings.Builder

	b.WriteString(f.theme.Title.Render(f.title))
	b.WriteString("\n\n")

	switch f.stage {
	case stagePreview:
		for _, line := range f.preview {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(f.theme.Hint.Render("equivalent CLI: " + f.cli))
		b.WriteString("\n")
		b.WriteString(f.theme.Hint.Render("enter run · esc back"))
	case stageExec:
		if f.hasExec {
			b.WriteString(stripScreenANSI(f.exec.View().Content))
		}
	default:
		if f.form != nil {
			b.WriteString(stripScreenANSI(f.form.View()))
			b.WriteString("\n")
		}
		b.WriteString(f.theme.Hint.Render("enter next · esc back"))
	}

	return b.String()
}

// stripScreenANSI drops CSI escape sequences so embedded huh and exec views
// snapshot as content. The tui kit owns its own copy for its package; this
// one serves package main screens.
func stripScreenANSI(s string) string {
	var b strings.Builder

	inEsc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEsc {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				inEsc = false
			}

			continue
		}
		if c == 0x1b {
			inEsc = true

			continue
		}

		b.WriteByte(c)
	}

	out := b.String()
	out = strings.ReplaceAll(out, "\a", "")

	return out
}

// validateIdentField rejects traversal content through the same
// isTraversalName/ErrPathTraversal path the cores use, then requires a .zen
// identifier. It runs as huh Validate (inline), not as a Go error.
func validateIdentField(v string) error {
	if isTraversalName(v) {
		return fmt.Errorf("%w: %q", ErrPathTraversal, v)
	}
	if !isIdent(v) {
		return fmt.Errorf("must be identifier, got %q", v)
	}

	return nil
}

// validateAppNameField applies the `zever new` app-name rule (letters,
// digits, - and _), which also confines the scaffold root against
// path traversal.
func validateAppNameField(v string) error {
	if !isValidAppName(v) {
		return fmt.Errorf("letters, digits, - and _ only, got %q", v)
	}

	return nil
}

// validateOptionalIdentField accepts empty (caller applies the default) and
// otherwise applies validateIdentField.
func validateOptionalIdentField(v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}

	return validateIdentField(strings.TrimSpace(v))
}

// validatePackageNameField rejects traversal content, then requires a Go
// package name for `generate adapter`.
func validatePackageNameField(v string) error {
	if isTraversalName(v) {
		return fmt.Errorf("%w: %q", ErrPathTraversal, v)
	}
	if !isPackageName(v) {
		return fmt.Errorf("lowercase letters and digits, starting with a letter, got %q", v)
	}

	return nil
}

// validateCronField mirrors GenerateSchedule's own cron gate so the form
// refuses quotes, backslashes and newlines inline.
func validateCronField(v string) error {
	if strings.TrimSpace(v) == "" {
		return errors.New("cron must not be empty")
	}
	if strings.ContainsAny(v, "\"\\\n") {
		return fmt.Errorf("must not contain quotes, backslashes or newlines, got %q", v)
	}

	return nil
}

// parseEntityFieldList parses a comma-separated "name:type" list through the
// same fieldSpecs.Set gate `generate entity` uses. Empty means no fields.
func parseEntityFieldList(s string) ([]EntityField, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	var specs fieldSpecs
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if err := specs.Set(part); err != nil {
			return nil, err
		}
	}

	out := make([]EntityField, 0, len(specs))
	for _, f := range specs {
		out = append(out, EntityField(f))
	}

	return out, nil
}

// validateEntityFieldList adapts parseEntityFieldList to huh Validate.
func validateEntityFieldList(s string) error {
	_, err := parseEntityFieldList(s)

	return err
}

// parseAdapterFieldList parses a comma-separated "key:type" list through the
// same optionSpecs.Set gate `generate adapter` uses. Empty means no fields.
func parseAdapterFieldList(s string) ([]AdapterOption, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	var specs optionSpecs
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if err := specs.Set(part); err != nil {
			return nil, err
		}
	}

	out := make([]AdapterOption, 0, len(specs))
	for _, f := range specs {
		out = append(out, AdapterOption(f))
	}

	return out, nil
}

// validateAdapterFieldList adapts parseAdapterFieldList to huh Validate.
func validateAdapterFieldList(s string) error {
	_, err := parseAdapterFieldList(s)

	return err
}

// cliQuote quotes one CLI word only when it carries whitespace or quotes,
// keeping equivalent-CLI lines copy-pasteable.
func cliQuote(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\n\"'") {
		return strconv.Quote(s)
	}

	return s
}

// cliFlag renders " --flag value", omitting the flag when value is empty.
func cliFlag(flag, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	return " --" + flag + " " + cliQuote(strings.TrimSpace(value))
}

// previewRow renders one "key: value" preview line.
func previewRow(key, value string) string {
	if strings.TrimSpace(value) == "" {
		value = "(default)"
	}

	return "  " + key + ": " + value
}

// RegisterScaffoldScreens exposes this group's dashboard entries for Wave 4
// wiring: group, name, description, exact equivalent CLI, and the owning
// screen constructor name. No models are built here.
func RegisterScaffoldScreens() []tui.Entry {
	return []tui.Entry{
		{Group: tui.GroupScaffold, Name: "new", Desc: "scaffold a new service", CLI: "zever new", Screen: "NewScaffoldScreen"},
		{Group: tui.GroupScaffold, Name: "generate", Desc: "generate code from schema", CLI: "zever generate", Screen: "NewGenerateScreen"},
		{Group: tui.GroupScaffold, Name: "extract", Desc: "extract a module into a standalone service", CLI: "zever extract", Screen: "NewExtractScreen"},
	}
}
