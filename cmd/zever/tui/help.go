package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// HelpCloseMsg is produced when the help overlay requests closing
// (esc, ? or q pressed while visible). The parent owns visibility and
// handles this message by hiding the overlay.
type HelpCloseMsg struct{}

// HelpModel is the `?` overlay: it renders the shared Keymap so help text
// is generated from the same value that drives dispatch. Pure render;
// the parent dashboard owns visibility state.
type HelpModel struct {
	keys  Keymap
	theme Theme
	width int
}

// NewHelp returns a help overlay bound to keys. Width defaults to 80 and
// follows WindowSizeMsg.
func NewHelp(keys Keymap) HelpModel {
	return HelpModel{keys: keys, theme: NewTheme(), width: 80}
}

// NewHelpWithTheme is NewHelp with an explicit theme.
func NewHelpWithTheme(keys Keymap, theme Theme) HelpModel {
	return HelpModel{keys: keys, theme: theme, width: 80}
}

// Init implements tea.Model. No I/O.
func (m HelpModel) Init() tea.Cmd { return nil }

// Update implements tea.Model. It sizes to the window and converts
// esc/?/q into HelpCloseMsg; all other messages are ignored (nil cmd).
// No I/O: pure state transition.
func (m HelpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "?", "q":
			return m, func() tea.Msg { return HelpCloseMsg{} }
		}
	}
	return m, nil
}

// Content renders the key table as a plain string for embedding in a
// parent view (e.g. dashboard overlay). Pure.
func (m HelpModel) Content() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("Keys"))
	b.WriteString("\n\n")
	for _, row := range m.keys.Rows() {
		b.WriteString(m.theme.Hint.Render(row.Keys[0]))
		b.WriteString("  ")
		b.WriteString(row.Desc)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// View implements tea.Model. Pure.
func (m HelpModel) View() tea.View {
	return tea.NewView(m.theme.Box.Render(m.Content()))
}
