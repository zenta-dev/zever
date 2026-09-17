package tui

import (
	"charm.land/lipgloss/v2"
)

// Theme is the single shared lipgloss style set for the zever TUI.
// Every screen renders through a Theme value so roles stay consistent and
// dirty per-screen styles cannot diverge.
//
// Color handling: lipgloss auto-degrades on non-TTY / limited color
// profiles, so no manual TTY detection is needed here. Keep styles free of
// I/O; construct once via NewTheme and reuse.
//
// Role inventory (absorbs the roles previously scattered across per-screen
// style.go files):
//
//	Title   - screen and dashboard headings
//	Cmd     - command names, selected entry
//	Dim     - secondary text, footers
//	Success - ok results
//	Fail    - errors
//	Hint    - key hints, "equivalent CLI" line, help overlay keys
//	Box     - bordered containers (help overlay, result panels)
//	Spinner - spinner glyph frame
type Theme struct {
	Title   lipgloss.Style
	Cmd     lipgloss.Style
	Dim     lipgloss.Style
	Success lipgloss.Style
	Fail    lipgloss.Style
	Hint    lipgloss.Style
	Box     lipgloss.Style
	Spinner lipgloss.Style
}

// NewTheme returns the default zever TUI theme. Pure: allocates styles only.
func NewTheme() Theme {
	return Theme{
		Title:   lipgloss.NewStyle().Bold(true),
		Cmd:     lipgloss.NewStyle().Bold(true),
		Dim:     lipgloss.NewStyle().Faint(true),
		Success: lipgloss.NewStyle().Bold(true),
		Fail:    lipgloss.NewStyle().Bold(true),
		Hint:    lipgloss.NewStyle().Faint(true),
		Box:     lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.RoundedBorder()),
		Spinner: lipgloss.NewStyle().Bold(true),
	}
}
