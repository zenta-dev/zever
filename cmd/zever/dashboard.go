// Dashboard wiring: TUI-first entry point for bare `zever` on a TTY.
//
// Mechanism (single program, model swap): runDashboard builds one
// shellModel over the dashboard entries and runs exactly one
// tea.Program. Selecting an entry swaps shellModel.active to the resolved
// screen (screenFor); the screen's esc-back contract (tui.BackMsg,
// tui.CanceledMsg, tui.LogBackMsg — see tui/run.go, tui/logpane.go and the
// screen_* esc paths) pops back to the dashboard by clearing active.
// Nested tea programs are deliberately NOT used: one program owns the
// terminal, so child screens never fight over input/output or leave the
// tty in a half-restored state.
//
// Exit status: 0 on clean quit (dashboard quit key, or esc at dashboard
// root), 1 when the program itself fails or a Screen name cannot be
// resolved. Screen errors never set the process status directly: they
// render inside the screen's exec/error view and the user backs out to
// the dashboard.
package main

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// dashboardEntries concatenates the four screen-group registrations in
// usage order. It mirrors printUsage's Scaffold/Inspect/Runtime/Database
// groups so `--help` and the dashboard show the same map.
func dashboardEntries() []tui.Entry {
	entries := RegisterScaffoldScreens()
	entries = append(entries, RegisterInspectScreens()...)
	entries = append(entries, RegisterRuntimeScreens()...)
	entries = append(entries, RegisterDatabaseScreens()...)

	return entries
}

// screenFor maps a dashboard entry's Screen constructor name to the live
// screen model. Unknown names are descriptive zever: errors, never panics.
func screenFor(entry tui.Entry) (tea.Model, error) {
	switch entry.Screen {
	case "NewScaffoldScreen":
		return NewScaffoldScreen(), nil
	case "NewGenerateScreen":
		return NewGenerateScreen(), nil
	case "NewExtractScreen":
		return NewExtractScreen(), nil
	case "NewCompileScreen":
		return NewCompileScreen(), nil
	case "NewCheckScreen":
		return NewCheckScreen(), nil
	case "NewBreakingScreen":
		return NewBreakingScreen(), nil
	case "NewFmtScreen":
		return NewFmtScreen(), nil
	case "NewDoctorScreen":
		return NewDoctorScreen(), nil
	case "NewConfigScreen":
		return NewConfigScreen(), nil
	case "NewRoutesScreen":
		return NewRoutesScreen(), nil
	case "NewExplainScreen":
		return NewExplainScreen(), nil
	case "NewBoundariesScreen":
		return NewBoundariesScreen(), nil
	case "NewGraphScreen":
		return NewGraphScreen(), nil
	case "NewServeScreen":
		return NewServeScreen(), nil
	case "NewDevScreen":
		return NewDevScreen(), nil
	case "NewQueueWorkScreen":
		return NewQueueWorkScreen(), nil
	case "NewTinkerScreen":
		return NewTinkerScreen(), nil
	case "NewMigrateScreen":
		return NewMigrateScreen(), nil
	case "NewRollbackScreen":
		return NewRollbackScreen(), nil
	case "NewSeedScreen":
		return NewSeedScreen(), nil
	default:
		return nil, fmt.Errorf("zever: unknown dashboard screen %q for entry %q", entry.Screen, entry.Name)
	}
}

// shellModel is the dashboard shell: the grouped command list plus at most
// one active screen. Zero active means the dashboard shows; resolving a
// SelectedMsg swaps the screen in, and the esc-back contract swaps it out.
type shellModel struct {
	dash   tui.DashboardModel
	active tea.Model
	err    error
}

// newShell returns the dashboard shell over entries (nil → defaults).
// Pure: no I/O.
func newShell(entries []tui.Entry) shellModel {
	return shellModel{dash: tui.NewDashboard(entries)}
}

// Init implements tea.Model. No I/O.
func (s shellModel) Init() tea.Cmd { return s.dash.Init() }

// Update implements tea.Model. No I/O: screens run their work in Cmds.
func (s shellModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.SelectedMsg:
		if s.active != nil {
			return s, nil
		}
		screen, err := screenFor(msg.Entry)
		if err != nil {
			s.err = err

			return s, tea.Quit
		}
		s.active = screen

		return s, screen.Init()
	case tui.BackMsg, tui.CanceledMsg, tui.LogBackMsg:
		if s.active != nil {
			s.active = nil

			return s, nil
		}
		// Esc at the dashboard root has nothing to pop: quit cleanly.
		return s, tea.Quit
	case tea.WindowSizeMsg:
		if s.active != nil {
			updated, cmd := s.active.Update(msg)
			s.active = updated

			return s, cmd
		}
		updated, cmd := s.dash.Update(msg)
		if dm, ok := updated.(*tui.DashboardModel); ok {
			s.dash = *dm
		}

		return s, cmd
	default:
	}

	if s.active != nil {
		updated, cmd := s.active.Update(msg)
		s.active = updated

		return s, cmd
	}
	updated, cmd := s.dash.Update(msg)
	if dm, ok := updated.(*tui.DashboardModel); ok {
		s.dash = *dm
	}

	return s, cmd
}

// View implements tea.Model. Pure.
func (s shellModel) View() tea.View {
	if s.active != nil {
		return s.active.View()
	}

	return s.dash.View()
}

// runShellProgram runs the dashboard shell as a single terminal program.
//
// Seam var so tests can cover runDashboard's post-run branches without a
// real TTY. Proof of behavior preservation: the default owns the terminal
// exactly as the inlined tea.NewProgram(newShell(entries)).Run() did — same
// program constructor, same model, same return — so every reachable input
// takes the identical path with or without this seam.
var runShellProgram = func(entries []tui.Entry) (tea.Model, error) {
	return tea.NewProgram(newShell(entries)).Run()
}

// dashboardEntrySource lists the dashboard entries.
//
// Seam var so tests can prove runDashboard rejects unresolvable entries
// without a TTY. Proof of behavior preservation: the default is exactly
// dashboardEntries — same function, same result — so every reachable input
// takes the identical path with or without this seam.
var dashboardEntrySource = dashboardEntries

// runDashboard validates every entry resolves, then runs the single shell
// program. A resolve failure before or during the session is a zever:
// error; a program failure is wrapped the same way.
func runDashboard() error {
	for _, e := range dashboardEntrySource() {
		if _, err := screenFor(e); err != nil {
			return err
		}
	}

	final, err := runShellProgram(dashboardEntrySource())
	if err != nil {
		return fmt.Errorf("zever: dashboard: %w", err)
	}
	if sh, ok := final.(shellModel); ok && sh.err != nil {
		return sh.err
	}

	return nil
}
