package main

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

func TestScreenFor_coversAllRegisteredEntries(t *testing.T) {
	for _, e := range dashboardEntries() {
		m, err := screenFor(e)
		if err != nil {
			t.Fatalf("screenFor(%+v) = %v", e, err)
		}
		if m == nil {
			t.Fatalf("screenFor(%+v) = nil model", e)
		}
	}
}

func TestScreenFor_unknownIsZeverError(t *testing.T) {
	_, err := screenFor(tui.Entry{Name: "bogus", Screen: "NewNopeScreen"})
	if err == nil {
		t.Fatal("screenFor(bogus) = nil, want error")
	}
	if !strings.HasPrefix(err.Error(), "zever:") {
		t.Fatalf("screenFor(bogus) = %q, want zever: prefix", err)
	}
}

func TestShell_selectPushesAndBackPops(t *testing.T) {
	sh := newShell([]tui.Entry{{Group: tui.GroupInspect, Name: "check", CLI: "zever check", Screen: "NewCheckScreen"}})
	updated, _ := sh.Update(tui.SelectedMsg{Entry: tui.Entry{Name: "check", Screen: "NewCheckScreen"}})
	sm, ok := updated.(shellModel)
	if !ok {
		t.Fatalf("Update = %T, want shellModel", updated)
	}
	if sm.active == nil {
		t.Fatal("SelectedMsg did not swap in a screen")
	}

	updated, _ = sm.Update(tui.BackMsg{})
	sm, ok = updated.(shellModel)
	if !ok {
		t.Fatalf("Update = %T, want shellModel", updated)
	}
	if sm.active != nil {
		t.Fatal("BackMsg did not pop back to dashboard")
	}
}

func TestShell_unknownScreenQuitsWithErr(t *testing.T) {
	sh := newShell([]tui.Entry{{Group: tui.GroupInspect, Name: "check", CLI: "zever check", Screen: "NewCheckScreen"}})
	updated, cmd := sh.Update(tui.SelectedMsg{Entry: tui.Entry{Name: "bogus", Screen: "NewNopeScreen"}})
	sm, ok := updated.(shellModel)
	if !ok {
		t.Fatalf("Update = %T, want shellModel", updated)
	}
	if sm.err == nil {
		t.Fatal("unknown Screen must record err")
	}
	if cmd == nil {
		t.Fatal("unknown Screen must return a quit cmd")
	}
}

func TestDashboardEntries_coversFourGroups(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range dashboardEntries() {
		seen[e.Group] = true
	}
	for _, g := range []string{tui.GroupScaffold, tui.GroupInspect, tui.GroupRuntime, tui.GroupDatabase} {
		if !seen[g] {
			t.Fatalf("dashboardEntries missing group %q", g)
		}
	}
}

// ---- shell Init/View/Update seams (no real TTY, bounded) ----

// fakeShellScreen is a canned active screen: records forwarded Msgs and
// renders a fixed view. Proves shellModel forwards without real screens.
type fakeShellScreen struct {
	seen []tea.Msg
	view string
}

func (f *fakeShellScreen) Init() tea.Cmd { return nil }

func (f *fakeShellScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	f.seen = append(f.seen, msg)
	return f, nil
}

func (f *fakeShellScreen) View() tea.View { return tea.NewView(f.view) }

// stubShellProgram replaces the runShellProgram seam for a test.
func stubShellProgram(t *testing.T, fn func([]tui.Entry) (tea.Model, error)) {
	t.Helper()

	prev := runShellProgram
	t.Cleanup(func() { runShellProgram = prev })
	runShellProgram = fn
}

func TestShell_Init_delegatesToDash(t *testing.T) {
	sh := newShell([]tui.Entry{{Group: tui.GroupInspect, Name: "check", CLI: "zever check", Screen: "NewCheckScreen"}})
	if cmd := sh.Init(); cmd != nil {
		t.Fatalf("shell Init = non-nil cmd, want nil (dash Init is nil)")
	}
}

func TestShell_View_dashboard(t *testing.T) {
	sh := newShell([]tui.Entry{
		{Group: tui.GroupInspect, Name: "check", Desc: "validate schemas", CLI: "zever check", Screen: "NewCheckScreen"},
	})

	got := sh.View().Content
	for _, want := range []string{"zever", "check", "validate schemas"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dashboard View lacks %q:\n%s", want, got)
		}
	}
}

func TestShell_View_active(t *testing.T) {
	sh := newShell([]tui.Entry{{Group: tui.GroupInspect, Name: "check", CLI: "zever check", Screen: "NewCheckScreen"}})
	sh.active = &fakeShellScreen{view: "canned-screen"}

	if got := sh.View().Content; got != "canned-screen" {
		t.Fatalf("active View = %q, want canned-screen", got)
	}
}

func TestShell_selectedWhileActive_ignored(t *testing.T) {
	sh := newShell([]tui.Entry{{Group: tui.GroupInspect, Name: "check", CLI: "zever check", Screen: "NewCheckScreen"}})
	first := &fakeShellScreen{view: "first"}
	sh.active = first

	updated, cmd := sh.Update(tui.SelectedMsg{Entry: tui.Entry{Name: "x", Screen: "NewCheckScreen"}})
	sm, ok := updated.(shellModel)
	if !ok {
		t.Fatalf("Update = %T, want shellModel", updated)
	}

	if sm.active != first {
		t.Fatal("second SelectedMsg while active must not swap screens")
	}

	if cmd != nil {
		t.Fatal("ignored SelectedMsg must return nil cmd")
	}
}

func TestShell_backVariants_atRootQuit(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.Msg
	}{
		{"back", tui.BackMsg{}},
		{"canceled", tui.CanceledMsg{}},
		{"logback", tui.LogBackMsg{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sh := newShell([]tui.Entry{{Group: tui.GroupInspect, Name: "check", CLI: "zever check", Screen: "NewCheckScreen"}})

			_, cmd := sh.Update(tc.msg)
			if cmd == nil {
				t.Fatalf("%T at root: want quit cmd, got nil", tc.msg)
			}

			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("%T at root: want QuitMsg, got %T", tc.msg, cmd())
			}
		})
	}
}

func TestShell_backVariants_popActive(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.Msg
	}{
		{"back", tui.BackMsg{}},
		{"canceled", tui.CanceledMsg{}},
		{"logback", tui.LogBackMsg{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sh := newShell([]tui.Entry{{Group: tui.GroupInspect, Name: "check", CLI: "zever check", Screen: "NewCheckScreen"}})
			sh.active = &fakeShellScreen{}

			updated, _ := sh.Update(tc.msg)
			sm, ok := updated.(shellModel)
			if !ok {
				t.Fatalf("Update = %T, want shellModel", updated)
			}

			if sm.active != nil {
				t.Fatalf("%T must pop active screen", tc.msg)
			}
		})
	}
}

func TestShell_windowSize_activeAndDash(t *testing.T) {
	t.Run("forwards to active, keeps it", func(t *testing.T) {
		fake := &fakeShellScreen{}
		sh := newShell([]tui.Entry{{Group: tui.GroupInspect, Name: "check", CLI: "zever check", Screen: "NewCheckScreen"}})
		sh.active = fake

		updated, _ := sh.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		sm, ok := updated.(shellModel)
		if !ok {
			t.Fatalf("Update = %T, want shellModel", updated)
		}

		if sm.active == nil {
			t.Fatal("WindowSizeMsg must keep the active screen")
		}

		found := false

		for _, m := range fake.seen {
			if _, ok := m.(tea.WindowSizeMsg); ok {
				found = true
			}
		}

		if !found {
			t.Fatal("WindowSizeMsg must forward to the active screen")
		}
	})

	t.Run("forwards to dash, still renders", func(t *testing.T) {
		sh := newShell([]tui.Entry{
			{Group: tui.GroupInspect, Name: "check", Desc: "validate", CLI: "zever check", Screen: "NewCheckScreen"},
		})

		updated, _ := sh.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		sm, ok := updated.(shellModel)
		if !ok {
			t.Fatalf("Update = %T, want shellModel", updated)
		}

		if got := sm.View().Content; !strings.Contains(got, "check") {
			t.Fatalf("dash View after resize lacks entry:\n%s", got)
		}
	})
}

func TestShell_defaultForward_activeAndDash(t *testing.T) {
	t.Run("forwards to active, keeps it", func(t *testing.T) {
		fake := &fakeShellScreen{}
		sh := newShell([]tui.Entry{{Group: tui.GroupInspect, Name: "check", CLI: "zever check", Screen: "NewCheckScreen"}})
		sh.active = fake

		updated, _ := sh.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		sm, ok := updated.(shellModel)
		if !ok {
			t.Fatalf("Update = %T, want shellModel", updated)
		}

		if sm.active == nil {
			t.Fatal("default msg must keep the active screen")
		}

		if len(fake.seen) != 1 {
			t.Fatalf("active saw %d msgs, want 1", len(fake.seen))
		}
	})

	t.Run("forwards to dash, cursor moves", func(t *testing.T) {
		sh := newShell([]tui.Entry{
			{Group: tui.GroupInspect, Name: "a", Screen: "NewCheckScreen"},
			{Group: tui.GroupInspect, Name: "b", Screen: "NewCheckScreen"},
		})

		updated, _ := sh.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		sm, ok := updated.(shellModel)
		if !ok {
			t.Fatalf("Update = %T, want shellModel", updated)
		}

		if got := sm.dash.Cursor(); got != 1 {
			t.Fatalf("dash cursor = %d, want 1 after j", got)
		}
	})
}

func TestRunDashboard_unresolvableEntry_error(t *testing.T) {
	prev := dashboardEntrySource
	t.Cleanup(func() { dashboardEntrySource = prev })
	dashboardEntrySource = func() []tui.Entry {
		return []tui.Entry{{Name: "bogus", Screen: "NewNopeScreen"}}
	}

	err := runDashboard()
	if err == nil {
		t.Fatal("runDashboard(bogus entry) = nil, want error")
	}

	if !strings.HasPrefix(err.Error(), "zever: unknown dashboard screen") {
		t.Fatalf("runDashboard(bogus entry) = %q, want unknown-screen error", err.Error())
	}
}

func TestRunDashboard_headlessProgramError(t *testing.T) {
	// Real default runner, no TTY: tea fails fast and runDashboard wraps.
	// Keeps the runShellProgram default body covered.
	err := runDashboard()
	if err == nil {
		t.Fatal("runDashboard headless = nil, want program error")
	}

	if !strings.HasPrefix(err.Error(), "zever: dashboard:") {
		t.Fatalf("runDashboard headless = %q, want zever: dashboard: wrap", err.Error())
	}
}

func TestRunDashboard_shellErr_returned(t *testing.T) {
	want := errors.New("boom-screen")
	stubShellProgram(t, func([]tui.Entry) (tea.Model, error) {
		return shellModel{err: want}, nil
	})

	if err := runDashboard(); !errors.Is(err, want) {
		t.Fatalf("runDashboard = %v, want shell err", err)
	}
}

func TestRunDashboard_cleanQuit_nil(t *testing.T) {
	stubShellProgram(t, func([]tui.Entry) (tea.Model, error) {
		return shellModel{}, nil
	})

	if err := runDashboard(); err != nil {
		t.Fatalf("runDashboard clean = %v, want nil", err)
	}
}
