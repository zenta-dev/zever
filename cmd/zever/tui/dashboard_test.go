package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func testEntries() []Entry {
	return []Entry{
		{Group: GroupScaffold, Name: "new", Desc: "scaffold a new service", CLI: "zever new", Screen: "NewScaffoldScreen"},
		{Group: GroupScaffold, Name: "generate", Desc: "generate code from schema", CLI: "zever generate", Screen: "NewGenerateScreen"},
		{Group: GroupInspect, Name: "check", Desc: "validate schemas", CLI: "zever check", Screen: "NewCheckScreen"},
		{Group: GroupRuntime, Name: "serve", Desc: "run the HTTP server", CLI: "zever serve", Screen: "NewServeScreen"},
		{Group: GroupRuntime, Name: "dev", Desc: "run with live reload", CLI: "zever dev", Screen: "NewDevScreen"},
		{Group: GroupDatabase, Name: "migrate", Desc: "run pending migrations", CLI: "zever db migrate", Screen: "NewMigrateScreen"},
		{Group: GroupDatabase, Name: "seed", Desc: "seed development data", CLI: "zever db seed", Screen: "NewSeedScreen"},
	}
}

func testDashboard() *DashboardModel {
	m := NewDashboard(testEntries())
	m.width, m.height = 80, 24
	return &m
}

func updateKey(m *DashboardModel, key tea.KeyPressMsg) (*DashboardModel, tea.Cmd) {
	got, cmd := m.Update(key)
	dm, ok := got.(*DashboardModel)
	if !ok {
		panic("updateKey: Update did not return *DashboardModel")
	}
	return dm, cmd
}

func TestDashboardExplicitEntries(t *testing.T) {
	m := testDashboard()
	if len(m.Entries()) != len(testEntries()) {
		t.Fatalf("entries = %d, want %d", len(m.Entries()), len(testEntries()))
	}
	if m.Cursor() != 0 || m.Filtering() || m.HelpVisible() {
		t.Fatalf("bad initial state: %+v", m)
	}
	if m.Init() != nil {
		t.Fatal("Init must return nil cmd")
	}
	seen := map[string]bool{}
	for _, e := range m.Entries() {
		if e.Group == "" || e.Name == "" || e.Desc == "" || e.Screen == "" {
			t.Fatalf("stub entry incomplete: %+v", e)
		}
		seen[e.Group] = true
	}
	for _, g := range GroupOrder {
		if !seen[g] {
			t.Fatalf("group %q missing from entries", g)
		}
	}
}

func TestDashboardNilIsEmpty(t *testing.T) {
	for _, entries := range [][]Entry{nil, {}} {
		m := NewDashboard(entries)
		if len(m.Entries()) != 0 {
			t.Fatalf("NewDashboard(%v) entries = %d, want 0 (no fallback)", entries, len(m.Entries()))
		}
		if len(m.Filtered()) != 0 {
			t.Fatalf("NewDashboard(%v) filtered = %d, want 0", entries, len(m.Filtered()))
		}
	}
}

func TestDashboardNavigate(t *testing.T) {
	m := testDashboard()
	n := len(m.Filtered())
	m, _ = updateKey(m, runeKey('j'))
	m, _ = updateKey(m, specialKey(tea.KeyDown))
	if m.Cursor() != 2 {
		t.Fatalf("cursor = %d, want 2", m.Cursor())
	}
	m, _ = updateKey(m, runeKey('k'))
	m, _ = updateKey(m, specialKey(tea.KeyUp))
	if m.Cursor() != 0 {
		t.Fatalf("cursor = %d, want 0", m.Cursor())
	}
	// Clamp at edges.
	m, _ = updateKey(m, runeKey('k'))
	if m.Cursor() != 0 {
		t.Fatalf("cursor below 0: %d", m.Cursor())
	}
	for i := 0; i < n+5; i++ {
		m, _ = updateKey(m, runeKey('j'))
	}
	if m.Cursor() != n-1 {
		t.Fatalf("cursor = %d, want last %d", m.Cursor(), n-1)
	}
}

func TestDashboardSelect(t *testing.T) {
	m := testDashboard()
	m, _ = updateKey(m, runeKey('j')) // generate
	m, cmd := updateKey(m, specialKey(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("enter must emit SelectedMsg")
	}
	sel, ok := cmd().(SelectedMsg)
	if !ok {
		t.Fatalf("cmd = %T, want SelectedMsg", cmd())
	}
	if sel.Entry.Name != "generate" {
		t.Fatalf("selected = %q, want generate", sel.Entry.Name)
	}
	if got := m.Recent(1); len(got) != 1 || got[0] != "generate" {
		t.Fatalf("history = %v", got)
	}
}

func TestDashboardBackAndQuit(t *testing.T) {
	m := testDashboard()
	_, cmd := updateKey(m, specialKey(tea.KeyEscape))
	if cmd == nil {
		t.Fatal("esc must emit BackMsg")
	}
	if _, ok := cmd().(BackMsg); !ok {
		t.Fatalf("cmd = %T, want BackMsg", cmd())
	}
	_, quit := updateKey(m, keypress('c', "", tea.ModCtrl))
	if quit == nil {
		t.Fatal("ctrl+c must quit")
	}
}

func TestDashboardHelpOverlay(t *testing.T) {
	m := testDashboard()
	m, _ = updateKey(m, runeKey('?'))
	if !m.HelpVisible() {
		t.Fatal("? must show help")
	}
	view := stripANSI(m.View().Content)
	if !containsStr(view, "move up") || !containsStr(view, "move down") {
		t.Fatalf("overlay missing keymap rows:\n%s", view)
	}
	m, cmd := updateKey(m, specialKey(tea.KeyEscape))
	if cmd == nil {
		t.Fatal("esc in help must produce a close message")
	}
	m2, _ := m.Update(cmd())
	m2dm, ok := m2.(*DashboardModel)
	if !ok {
		t.Fatalf("Update = %T, want *DashboardModel", m2)
	}
	m = m2dm
	if m.HelpVisible() {
		t.Fatal("esc must close help via HelpCloseMsg")
	}
	// Direct HelpCloseMsg path.
	m, _ = updateKey(m, runeKey('?'))
	got, _ := m.Update(HelpCloseMsg{})
	gotDM, ok := got.(*DashboardModel)
	if !ok {
		t.Fatalf("Update = %T, want *DashboardModel", got)
	}
	if gotDM.HelpVisible() {
		t.Fatal("HelpCloseMsg must hide help")
	}
}

func TestDashboardFilter(t *testing.T) {
	m := testDashboard()
	m, _ = updateKey(m, runeKey('/'))
	if !m.Filtering() {
		t.Fatal("/ must enter filter mode")
	}
	for _, r := range "serve" {
		m, _ = updateKey(m, runeKey(r))
	}
	if m.FilterText() != "serve" {
		t.Fatalf("filter = %q", m.FilterText())
	}
	list := m.Filtered()
	if len(list) != 1 || list[0].Name != "serve" {
		t.Fatalf("filtered = %+v", list)
	}
	// Enter selects the filtered hit and records history.
	_, cmd := updateKey(m, specialKey(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("enter in filter must select")
	}
	sel, ok := cmd().(SelectedMsg)
	if !ok {
		t.Fatalf("cmd = %T, want SelectedMsg", cmd())
	}
	if sel.Entry.Name != "serve" {
		t.Fatalf("selected = %q", sel.Entry.Name)
	}
}

func TestDashboardFilterNavigate(t *testing.T) {
	m := testDashboard()
	m, _ = updateKey(m, runeKey('/'))
	m, _ = updateKey(m, runeKey('e')) // matches many entries
	if len(m.Filtered()) < 3 {
		t.Fatalf("want multi-match filter, got %+v", m.Filtered())
	}
	m, _ = updateKey(m, runeKey('j'))
	m, _ = updateKey(m, specialKey(tea.KeyDown))
	if m.Cursor() != 2 {
		t.Fatalf("filter cursor = %d, want 2", m.Cursor())
	}
	m, _ = updateKey(m, runeKey('k'))
	if m.Cursor() != 1 {
		t.Fatalf("filter cursor = %d, want 1", m.Cursor())
	}
	// Non-text control key in filter mode is ignored.
	_, cmd := updateKey(m, keypress('c', "", tea.ModCtrl))
	if cmd != nil {
		t.Fatal("ctrl+c in filter must not act")
	}
}

func TestDashboardFilterBackspaceAndExit(t *testing.T) {
	m := testDashboard()
	m, _ = updateKey(m, runeKey('/'))
	if !m.Filtering() {
		t.Fatal("must be in filter mode")
	}
	m, _ = updateKey(m, runeKey('x'))
	m, _ = updateKey(m, specialKey(tea.KeyBackspace))
	if m.FilterText() != "" {
		t.Fatalf("filter = %q, want empty", m.FilterText())
	}
	if len(m.Filtered()) != len(m.Entries()) {
		t.Fatal("empty filter must match all")
	}
	m, _ = updateKey(m, specialKey(tea.KeyEscape))
	if m.Filtering() || m.FilterText() != "" {
		t.Fatalf("esc must exit filter: %+v", m)
	}
}

func TestDashboardFilterNoMatch(t *testing.T) {
	m := testDashboard()
	m, _ = updateKey(m, runeKey('/'))
	for _, r := range "zzz" {
		m, _ = updateKey(m, runeKey(r))
	}
	if len(m.Filtered()) != 0 {
		t.Fatalf("filtered = %+v, want empty", m.Filtered())
	}
	view := stripANSI(m.View().Content)
	if !containsStr(view, "no matches") {
		t.Fatalf("view missing empty-state:\n%s", view)
	}
	// Enter on empty list is a no-op.
	_, cmd := updateKey(m, specialKey(tea.KeyEnter))
	if cmd != nil {
		t.Fatal("enter on empty filter must be no-op")
	}
}

func TestDashboardHistoryDedupeAndCap(t *testing.T) {
	m := testDashboard()
	m.Record("serve")
	m.Record("dev")
	m.Record("serve")
	if got := m.Recent(10); len(got) != 2 || got[0] != "serve" || got[1] != "dev" {
		t.Fatalf("recent = %v", got)
	}
	for i := 0; i < 15; i++ {
		m.Record("cmd" + strings.Repeat("x", i))
	}
	if len(m.Recent(100)) != 10 {
		t.Fatalf("history uncapped: %d", len(m.Recent(100)))
	}
	m.Record("")
	if len(m.Recent(100)) != 10 {
		t.Fatal("empty record must be ignored")
	}
}

func TestDashboardResize(t *testing.T) {
	m := testDashboard()
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	dm, ok := got.(*DashboardModel)
	if !ok {
		t.Fatalf("Update = %T, want *DashboardModel", got)
	}
	if dm.width != 100 || dm.height != 30 {
		t.Fatalf("size = %dx%d", dm.width, dm.height)
	}
}

func TestDashboardGolden80x24(t *testing.T) {
	m := testDashboard() // 80x24 pinned
	got := stripANSI(m.View().Content)
	want := `zever

Scaffold
▸ new  scaffold a new service
  generate  generate code from schema

Inspect
  check  validate schemas

Runtime
  serve  run the HTTP server
  dev  run with live reload

Database
  migrate  run pending migrations
  seed  seed development data

j/k move · enter select · / filter · ? help · esc back · ctrl+c quit`
	if got != want {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestDashboardViewShowsHistory(t *testing.T) {
	m := testDashboard()
	m, _ = updateKey(m, specialKey(tea.KeyEnter)) // select "new"
	view := stripANSI(m.View().Content)
	if !containsStr(view, "recent: new") {
		t.Fatalf("view missing history line:\n%s", view)
	}
}
