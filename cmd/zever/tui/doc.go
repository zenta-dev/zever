// Package tui is the shared terminal UI kit and dashboard shell for the
// TUI-first zever command surface.
//
// The kit owns cross-screen concerns only: a single lipgloss theme
// (theme.go), a single keymap that drives both dispatch and help text
// (keymap.go, help.go), an execute-with-spinner scaffold (run.go), a
// scrolling log viewport (logpane.go), and the grouped command dashboard
// (dashboard.go).
//
// Per-screen agents build their screens on these primitives. Dispatch and
// help must share the Keymap value so help text can never drift from the
// keys Update actually handles. View methods are pure: no I/O in Update;
// long work runs in Cmds that produce Msgs.
package tui
