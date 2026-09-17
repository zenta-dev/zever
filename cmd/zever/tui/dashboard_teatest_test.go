package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
)

// TestDashboardProgramNavigateQuit runs the dashboard as a real program:
// navigate down twice, then quit. Proves wiring beyond unit Update calls.
func TestDashboardProgramNavigateQuit(t *testing.T) {
	m := NewDashboard([]Entry{
		{Group: GroupScaffold, Name: "new", Desc: "scaffold a new service", CLI: "zever new", Screen: "NewScaffoldScreen"},
		{Group: GroupScaffold, Name: "generate", Desc: "generate code from schema", CLI: "zever generate", Screen: "NewGenerateScreen"},
		{Group: GroupInspect, Name: "check", Desc: "validate schemas", CLI: "zever check", Screen: "NewCheckScreen"},
	})
	tm := teatest.NewTestModel(t, &m)
	tm.Send(specialKey(tea.KeyDown))
	tm.Send(runeKey('j'))
	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	tm.WaitFinished(t)
	final, ok := tm.FinalModel(t).(*DashboardModel)
	if !ok {
		t.Fatalf("final model = %T, want *DashboardModel", tm.FinalModel(t))
	}
	if final.Cursor() != 2 {
		t.Fatalf("cursor = %d, want 2", final.Cursor())
	}
}
