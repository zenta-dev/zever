package tui

import (
	"context"
	"errors"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// ErrCanceled reports user cancellation of a running ExecModel (esc).
// It is a LOCAL sentinel for the tui package: the main zever command
// package owns the canonical error set, so screen agents must translate
// ErrCanceled at the handoff boundary (map to the main-package
// cancellation error when reporting outward).
var ErrCanceled = errors.New("tui: canceled")

// ExecState is the lifecycle phase of an ExecModel.
type ExecState int

const (
	// ExecRunning: work in flight, spinner shown.
	ExecRunning ExecState = iota
	// ExecDone: work finished successfully, result view shown.
	ExecDone
	// ExecFailed: work finished with error, error view shown.
	ExecFailed
	// ExecCanceled: user pressed esc, cancellation view shown.
	ExecCanceled
)

// ExecFunc is the unit of work an ExecModel runs. It executes inside a
// tea.Cmd (off the Update path) and must respect ctx cancellation.
// It returns the human-readable result text shown in the result view.
type ExecFunc func(ctx context.Context) (string, error)

// ExecDoneMsg carries successful completion of the ExecFunc.
type ExecDoneMsg struct{ Output string }

// ExecErrMsg carries failure of the ExecFunc. A wrapped ErrCanceled is
// normalized to ExecCanceled state, not ExecFailed.
type ExecErrMsg struct{ Err error }

// CanceledMsg notifies the parent that the user canceled execution.
// Parents that push the next screen on selection use this to pop back.
type CanceledMsg struct{}

// ExecModel runs one ExecFunc with a spinner, then shows the result view
// with an "equivalent CLI: <cmd>" learnability line, or the error view on
// failure. Cancellation via esc follows the errCanceled-style flow: the
// state moves to ExecCanceled and a CanceledMsg is emitted for the parent.
type ExecModel struct {
	title  string
	cli    string
	fn     ExecFunc
	state  ExecState
	output string
	err    error
	spin   spinner.Model
	theme  Theme
	keys   Keymap
}

// NewExec returns an ExecModel ready to run title via fn. cli is the
// equivalent CLI invocation shown in the result view (learnability
// requirement); it is display-only. Pure: no work starts until Start runs
// as a tea.Cmd.
func NewExec(title, cli string, fn ExecFunc) ExecModel {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	return ExecModel{
		title: title,
		cli:   cli,
		fn:    fn,
		state: ExecRunning,
		spin:  sp,
		theme: NewTheme(),
		keys:  DefaultKeymap(),
	}
}

// Title returns the display title.
func (m ExecModel) Title() string { return m.title }

// CLI returns the equivalent CLI string shown in the result view.
func (m ExecModel) CLI() string { return m.cli }

// State returns the current lifecycle phase.
func (m ExecModel) State() ExecState { return m.state }

// Output returns the result text after ExecDone.
func (m ExecModel) Output() string { return m.output }

// Err returns the failure after ExecFailed.
func (m ExecModel) Err() error { return m.err }

// Init implements tea.Model. It starts the spinner tick; the caller must
// additionally schedule m.Start() (Batch both).
func (m ExecModel) Init() tea.Cmd { return m.spin.Tick }

// Start runs the ExecFunc and maps its return to ExecDoneMsg/ExecErrMsg.
// Run as a tea.Cmd: no I/O on the Update path.
func (m ExecModel) Start() tea.Msg {
	if m.fn == nil {
		return ExecDoneMsg{}
	}
	out, err := m.fn(context.Background())
	if err != nil {
		return ExecErrMsg{Err: err}
	}
	return ExecDoneMsg{Output: out}
}

// Update implements tea.Model. Pure dispatch: spinner ticks advance the
// spinner, done/err messages flip state, esc cancels.
func (m ExecModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.keys.Back.Matches(msg.String()) && m.state == ExecRunning {
			m.state = ExecCanceled
			m.err = ErrCanceled
			return m, func() tea.Msg { return CanceledMsg{} }
		}
		return m, nil
	case ExecDoneMsg:
		m.state = ExecDone
		m.output = msg.Output
		return m, nil
	case ExecErrMsg:
		if errors.Is(msg.Err, ErrCanceled) || errors.Is(msg.Err, context.Canceled) {
			m.state = ExecCanceled
		} else {
			m.state = ExecFailed
		}
		m.err = msg.Err
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model. Pure.
func (m ExecModel) View() tea.View {
	var b strings.Builder
	switch m.state {
	case ExecRunning:
		b.WriteString(m.spin.View())
		b.WriteString(" ")
		b.WriteString(m.theme.Title.Render(m.title))
		b.WriteString("…\n")
		b.WriteString(m.theme.Hint.Render("esc cancel"))
	case ExecDone:
		b.WriteString(m.theme.Success.Render("done: "))
		b.WriteString(m.title)
		if m.output != "" {
			b.WriteString("\n\n")
			b.WriteString(m.output)
		}
		b.WriteString("\n\n")
		b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.cli))
		b.WriteString("\n")
		b.WriteString(m.theme.Hint.Render("enter continue · esc back"))
	case ExecFailed:
		b.WriteString(m.theme.Fail.Render("error: "))
		b.WriteString(m.title)
		b.WriteString("\n\n")
		if m.err != nil {
			b.WriteString(m.err.Error())
		}
		b.WriteString("\n\n")
		b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.cli))
		b.WriteString("\n")
		b.WriteString(m.theme.Hint.Render("esc back"))
	case ExecCanceled:
		b.WriteString(m.theme.Hint.Render("canceled: "))
		b.WriteString(m.title)
		b.WriteString("\n")
		b.WriteString(m.theme.Hint.Render("esc back"))
	}
	return tea.NewView(b.String())
}
