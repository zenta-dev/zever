package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// LogBackMsg requests the parent to pop the log pane (esc-back contract).
// LogModel never quits the program itself; the parent owns navigation.
type LogBackMsg struct{}

// LogModel is a scrolling viewport for serve/dev/tinker output with
// follow-tail semantics: while the view sits at the bottom, appended lines
// auto-scroll; once the user scrolls up, follow pauses until they return
// to the bottom. Pure View; appointment of content happens via Append
// (called from Update on the parent's Msgs, never doing I/O itself).
type LogModel struct {
	vp     viewport.Model
	follow bool
	theme  Theme
	keys   Keymap
	width  int
	height int
}

// NewLog returns a log pane sized w×h with follow-tail enabled.
// Non-positive dimensions fall back to 80×24.
func NewLog(w, h int) LogModel {
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	vp := viewport.New(viewport.WithWidth(w), viewport.WithHeight(h))
	return LogModel{vp: vp, follow: true, theme: NewTheme(), keys: DefaultKeymap(), width: w, height: h}
}

// Append adds lines to the pane. When follow-tail is active (or the view
// was already at the bottom), it scrolls to the newest line. Pure.
func (m *LogModel) Append(lines ...string) {
	if len(lines) == 0 {
		return
	}
	atBottom := m.vp.AtBottom()
	cur := m.vp.GetContent()
	var b strings.Builder
	if cur != "" {
		b.WriteString(cur)
		b.WriteString("\n")
	}
	b.WriteString(strings.Join(lines, "\n"))
	m.vp.SetContent(b.String())
	if m.follow || atBottom {
		m.vp.GotoBottom()
	}
}

// Lines returns the current pane content split per line.
func (m *LogModel) Lines() []string {
	if c := m.vp.GetContent(); c != "" {
		return strings.Split(c, "\n")
	}
	return nil
}

// Follow reports whether follow-tail is armed.
func (m *LogModel) Follow() bool { return m.follow }

// AtBottom reports whether the viewport shows the newest line.
func (m *LogModel) AtBottom() bool { return m.vp.AtBottom() }

// Init implements tea.Model. No I/O.
func (m *LogModel) Init() tea.Cmd { return nil }

// Update implements tea.Model. Viewport keys scroll; esc emits LogBackMsg
// per the esc-back contract (the parent pops). WindowSizeMsg resizes.
// No I/O: all content arrives via Append from parent-held Msgs.
func (m *LogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
			m.vp.SetWidth(msg.Width)
		}
		if msg.Height > 0 {
			m.height = msg.Height
			m.vp.SetHeight(msg.Height)
		}
		return m, nil
	case tea.KeyPressMsg:
		if m.keys.Back.Matches(msg.String()) {
			return m, func() tea.Msg { return LogBackMsg{} }
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	// Re-arm follow-tail when the user scrolls back to the bottom.
	m.follow = m.vp.AtBottom()
	return m, cmd
}

// View implements tea.Model. Pure.
func (m *LogModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.vp.View())
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("up/down scroll · esc back"))
	return tea.NewView(b.String())
}
