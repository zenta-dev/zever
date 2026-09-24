package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Dashboard groups mirror the CLI usage groups so the TUI and `--help`
// show the same map.
const (
	GroupScaffold = "Scaffold"
	GroupInspect  = "Inspect"
	GroupRuntime  = "Runtime"
	GroupDatabase = "Database"
)

// GroupOrder is the stable display order of dashboard groups.
var GroupOrder = []string{GroupScaffold, GroupInspect, GroupRuntime, GroupDatabase}

// Entry is one dashboard command row. Screen is the exact future screen
// constructor owned by the sibling screen agent; dashboard entries are
// STUBS (name + description + TODO mapping) with no logic.
type Entry struct {
	Group string
	Name  string
	Desc  string
	CLI   string
	// Screen names the future constructor, e.g. "NewServeScreen".
	// TODO(screen agents): replace the stub with a call to Screen,
	// keeping Name/Desc/CLI as the contract.
	Screen string
}

// DefaultEntries returns the stub dashboard entries. No logic lives here;
// each row maps to a future screen constructor (see Screen).
func DefaultEntries() []Entry {
	return []Entry{
		// TODO(screen agents): wire to NewScaffoldScreen.
		{Group: GroupScaffold, Name: "new", Desc: "scaffold a new service", CLI: "zever new", Screen: "NewScaffoldScreen"},
		{Group: GroupScaffold, Name: "generate", Desc: "generate code from schema", CLI: "zever generate", Screen: "NewGenerateScreen"},
		// Doctor and config screens (NewDoctorScreen, NewConfigScreen) are
		// real: they live in package main (cmd/zever/screen_inspect2.go)
		// and are resolved by cmd/zever/dashboard.go's screenFor, which
		// this tui package cannot import without a cycle. This entry list
		// is kept in sync with cmd/zever's RegisterInspectScreens by hand;
		// DefaultEntries itself is only a nil-entries fallback (tests),
		// never the live dashboard's entry source.
		{Group: GroupInspect, Name: "doctor", Desc: "check environment health", CLI: "zever doctor", Screen: "NewDoctorScreen"},
		{Group: GroupInspect, Name: "config", Desc: "inspect resolved config", CLI: "zever config show", Screen: "NewConfigScreen"},
		// TODO(screen agents): wire to NewRuntimeScreen.
		{Group: GroupRuntime, Name: "serve", Desc: "run the HTTP server", CLI: "zever serve", Screen: "NewServeScreen"},
		{Group: GroupRuntime, Name: "dev", Desc: "run with live reload", CLI: "zever dev", Screen: "NewDevScreen"},
		{Group: GroupRuntime, Name: "tinker", Desc: "open an interactive REPL", CLI: "zever tinker", Screen: "NewTinkerScreen"},
		// TODO(screen agents): wire to NewDatabaseScreen.
		{Group: GroupDatabase, Name: "migrate", Desc: "run pending migrations", CLI: "zever db migrate", Screen: "NewMigrateScreen"},
		{Group: GroupDatabase, Name: "seed", Desc: "seed development data", CLI: "zever db seed", Screen: "NewSeedScreen"},
	}
}

// SelectedMsg notifies the parent that the user picked an entry.
// The parent (cmd/zever/*.go runX) pushes the entry's Screen.
type SelectedMsg struct{ Entry Entry }

// BackMsg requests the parent to pop the dashboard (esc-back contract).
type BackMsg struct{}

// DashboardModel is the grouped command list shell: fuzzy filter,
// in-memory recent history, and help overlay wiring. Screen agents embed
// or wrap it; selection flows out via SelectedMsg.
type DashboardModel struct {
	entries    []Entry
	cursor     int
	filterOn   bool
	filterText string
	history    []string
	showHelp   bool
	help       HelpModel
	keys       Keymap
	theme      Theme
	width      int
	height     int
}

// NewDashboard returns a dashboard over entries. The caller must pass
// explicit entries (normally dashboardEntries in cmd/zever); nil is treated
// as empty with no fallback. Pure: no I/O.
func NewDashboard(entries []Entry) DashboardModel {
	if entries == nil {
		entries = []Entry{}
	}
	keys := DefaultKeymap()
	return DashboardModel{
		entries: entries,
		keys:    keys,
		theme:   NewTheme(),
		help:    NewHelp(keys),
		width:   80,
		height:  24,
	}
}

// Entries returns the full (unfiltered) entry list.
func (m *DashboardModel) Entries() []Entry { return m.entries }

// Cursor returns the cursor index into the current filtered list.
func (m *DashboardModel) Cursor() int { return m.cursor }

// Filtering reports whether filter input is active.
func (m *DashboardModel) Filtering() bool { return m.filterOn }

// FilterText returns the current filter string.
func (m *DashboardModel) FilterText() string { return m.filterText }

// HelpVisible reports whether the help overlay shows.
func (m *DashboardModel) HelpVisible() bool { return m.showHelp }

// Record pushes name onto the in-memory recent history (most recent
// first, de-duplicated, capped at 10). Persistence comes later.
func (m *DashboardModel) Record(name string) {
	if name == "" {
		return
	}
	out := make([]string, 0, 10)
	out = append(out, name)
	for _, h := range m.history {
		if h != name {
			out = append(out, h)
		}
	}
	if len(out) > 10 {
		out = out[:10]
	}
	m.history = out
}

// Recent returns up to n most-recent entry names (in-memory only).
func (m *DashboardModel) Recent(n int) []string {
	if n <= 0 || n > len(m.history) {
		n = len(m.history)
	}
	return append([]string(nil), m.history[:n]...)
}

// Filtered returns entries matching the filter (case-insensitive
// substring over name, description, and group). Empty filter matches all.
// Pure.
func (m *DashboardModel) Filtered() []Entry {
	q := strings.ToLower(strings.TrimSpace(m.filterText))
	if q == "" {
		return append([]Entry(nil), m.entries...)
	}
	var out []Entry
	for _, e := range m.entries {
		hay := strings.ToLower(e.Name + " " + e.Desc + " " + e.Group)
		if strings.Contains(hay, q) {
			out = append(out, e)
		}
	}
	return out
}

// Init implements tea.Model. No I/O.
func (m *DashboardModel) Init() tea.Cmd { return nil }

// selectAt records history and emits SelectedMsg for filtered index i.
func (m *DashboardModel) selectAt(list []Entry, i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(list) {
		return m, nil
	}
	m.Record(list[i].Name)
	entry := list[i]
	return m, func() tea.Msg { return SelectedMsg{Entry: entry} }
}

// Update implements tea.Model. Dispatch and help share m.keys (single
// source). No I/O: selections and navigation flow out as Msgs.
func (m *DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		hm, _ := m.help.Update(msg)
		if h, ok := hm.(HelpModel); ok {
			m.help = h
		}
		return m, nil
	case HelpCloseMsg:
		m.showHelp = false
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		if m.showHelp {
			hm, cmd := m.help.Update(msg)
			if h, ok := hm.(HelpModel); ok {
				m.help = h
			}
			return m, cmd
		}
		if m.filterOn {
			switch {
			case key == "esc":
				m.filterOn = false
				m.filterText = ""
				m.cursor = 0
				return m, nil
			case key == "enter":
				return m.selectAt(m.Filtered(), m.cursor)
			case key == "backspace":
				if len(m.filterText) > 0 {
					m.filterText = m.filterText[:len(m.filterText)-1]
					m.cursor = 0
				}
				return m, nil
			case m.keys.Up.Matches(key):
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil
			case m.keys.Down.Matches(key):
				if m.cursor < len(m.Filtered())-1 {
					m.cursor++
				}
				return m, nil
			default:
				if msg.Text != "" && msg.Mod == 0 {
					m.filterText += msg.Text
					m.cursor = 0
				}
				return m, nil
			}
		}
		switch {
		case m.keys.Quit.Matches(key):
			return m, tea.Quit
		case m.keys.Help.Matches(key):
			m.showHelp = true
			return m, nil
		case m.keys.Filter.Matches(key):
			m.filterOn = true
			m.filterText = ""
			m.cursor = 0
			return m, nil
		case m.keys.Up.Matches(key):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case m.keys.Down.Matches(key):
			if m.cursor < len(m.Filtered())-1 {
				m.cursor++
			}
			return m, nil
		case m.keys.Select.Matches(key):
			return m.selectAt(m.Filtered(), m.cursor)
		case m.keys.Back.Matches(key):
			return m, func() tea.Msg { return BackMsg{} }
		}
	}
	return m, nil
}

// View implements tea.Model. Pure. Fixed logical size follows the last
// WindowSizeMsg; tests pin 80x24.
func (m *DashboardModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("zever"))
	b.WriteString("\n\n")
	if m.filterOn {
		b.WriteString(m.theme.Hint.Render("/" + m.filterText + "▌"))
		b.WriteString("\n\n")
	}
	list := m.Filtered()
	for _, g := range GroupOrder {
		var rows []Entry
		var idx []int
		for i, e := range list {
			if e.Group == g {
				rows = append(rows, e)
				idx = append(idx, i)
			}
		}
		if len(rows) == 0 {
			continue
		}
		b.WriteString(m.theme.Dim.Render(g))
		b.WriteString("\n")
		for j, e := range rows {
			marker := "  "
			name := e.Name
			if idx[j] == m.cursor {
				marker = "▸ "
				name = m.theme.Cmd.Render(e.Name)
			}
			b.WriteString(marker + name)
			b.WriteString("  ")
			b.WriteString(m.theme.Dim.Render(e.Desc))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(list) == 0 {
		b.WriteString(m.theme.Hint.Render("no matches"))
		b.WriteString("\n\n")
	}
	if len(m.history) > 0 {
		b.WriteString(m.theme.Dim.Render("recent: " + strings.Join(m.history, ", ")))
		b.WriteString("\n\n")
	}
	b.WriteString(m.theme.Hint.Render("j/k move · enter select · / filter · ? help · esc back · ctrl+c quit"))
	content := strings.TrimRight(b.String(), "\n")
	if m.showHelp {
		content += "\n\n" + m.help.Content()
	}
	return tea.NewView(content)
}
