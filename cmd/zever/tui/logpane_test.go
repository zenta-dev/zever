package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNewLogDefaults(t *testing.T) {
	m := NewLog(0, 0)
	if m.Follow() != true {
		t.Fatal("follow-tail must default on")
	}
	if !m.AtBottom() {
		t.Fatal("empty pane must be at bottom")
	}
	if m.Lines() != nil {
		t.Fatalf("empty pane lines = %v", m.Lines())
	}
	lmInit := NewLog(80, 24)
	if lmInit.Init() != nil {
		t.Fatal("Init must return nil cmd")
	}
}

func TestLogAppendAndLines(t *testing.T) {
	m := NewLog(80, 10)
	m.Append("booting", "listening :8080")
	lines := m.Lines()
	if len(lines) != 2 || lines[0] != "booting" || lines[1] != "listening :8080" {
		t.Fatalf("lines = %v", lines)
	}
	if !m.AtBottom() {
		t.Fatal("append at bottom must follow tail")
	}
	m.Append()
	if len(m.Lines()) != 2 {
		t.Fatal("empty Append must be a no-op")
	}
}

func TestLogEscBackContract(t *testing.T) {
	m := NewLog(80, 10)
	m.Append("line")
	_, cmd := m.Update(specialKey(tea.KeyEscape))
	if cmd == nil {
		t.Fatal("esc must emit LogBackMsg cmd")
	}
	if _, ok := cmd().(LogBackMsg); !ok {
		t.Fatalf("cmd = %T, want LogBackMsg", cmd())
	}
}

func TestLogResize(t *testing.T) {
	m := NewLog(80, 10)
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	lm, ok := got.(*LogModel)
	if !ok {
		t.Fatalf("Update = %T, want *LogModel", got)
	}
	if lm.width != 100 || lm.height != 20 {
		t.Fatalf("size = %dx%d", lm.width, lm.height)
	}
}

func TestLogScrollReArmsFollow(t *testing.T) {
	m := NewLog(40, 3)
	for i := 0; i < 20; i++ {
		m.Append("line")
	}
	if !m.AtBottom() {
		t.Fatal("bulk append must end at bottom")
	}
	// Scroll up: viewport handles pgup; follow must disarm.
	updated, _ := m.Update(specialKey(tea.KeyPgUp))
	lm, ok := updated.(*LogModel)
	if !ok {
		t.Fatalf("Update = %T, want *LogModel", updated)
	}
	if lm.AtBottom() {
		t.Fatal("pgup must leave bottom")
	}
	if lm.Follow() {
		t.Fatal("follow must disarm once user scrolls up")
	}
	view := stripANSI(lm.View().Content)
	if !containsStr(view, "esc back") {
		t.Fatalf("log view missing hint:\n%s", view)
	}
}
