package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHelpInitNil(t *testing.T) {
	if NewHelp(DefaultKeymap()).Init() != nil {
		t.Fatal("Help Init must return nil cmd")
	}
}

func TestHelpCloseKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		specialKey(tea.KeyEscape),
		runeKey('?'),
		runeKey('q'),
	} {
		m := NewHelp(DefaultKeymap())
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Fatalf("key %q: want HelpCloseMsg cmd, got nil", key.String())
		}
		if _, ok := cmd().(HelpCloseMsg); !ok {
			t.Fatalf("key %q: want HelpCloseMsg, got %T", key.String(), cmd())
		}
	}
}

func TestHelpIgnoresOtherKeys(t *testing.T) {
	m := NewHelp(DefaultKeymap())
	_, cmd := m.Update(runeKey('j'))
	if cmd != nil {
		t.Fatal("non-close key must yield nil cmd")
	}
}

func TestHelpResize(t *testing.T) {
	m := NewHelp(DefaultKeymap())
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	hm, ok := got.(HelpModel)
	if !ok {
		t.Fatalf("Update = %T, want HelpModel", got)
	}
	if hm.width != 100 {
		t.Fatalf("width = %d, want 100", hm.width)
	}
}

func TestHelpContentListsKeymap(t *testing.T) {
	m := NewHelp(DefaultKeymap())
	got := stripANSI(m.Content())
	for _, row := range DefaultKeymap().Rows() {
		if !containsStr(got, row.Keys[0]) {
			t.Fatalf("help missing canonical key %q in:\n%s", row.Keys[0], got)
		}
		if !containsStr(got, row.Desc) {
			t.Fatalf("help missing desc %q in:\n%s", row.Desc, got)
		}
	}
}

func TestHelpGolden(t *testing.T) {
	m := NewHelp(DefaultKeymap())
	got := stripANSI(m.View().Content)
	if !containsStr(got, "Keys") {
		t.Fatalf("help view missing title:\n%s", got)
	}
	lines := splitLines(got)
	if len(lines) < 9 {
		t.Fatalf("help view too short (%d lines):\n%s", len(lines), got)
	}
	// First content line after the box top border is the title.
	if !containsStr(lines[1], "Keys") {
		t.Fatalf("help title not on second line:\n%s", got)
	}
}

func TestNewHelpWithTheme(t *testing.T) {
	th := NewTheme()
	m := NewHelpWithTheme(DefaultKeymap(), th)
	if stripANSI(m.Content()) == "" {
		t.Fatal("themed help must render")
	}
}
