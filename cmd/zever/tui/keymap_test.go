package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func keypress(code rune, text string, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text, Mod: mod}
}

func runeKey(r rune) tea.KeyPressMsg { return keypress(r, string(r), 0) }

func specialKey(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

func TestDefaultKeymapContract(t *testing.T) {
	k := DefaultKeymap()
	cases := map[string]struct {
		b   Binding
		key string
	}{
		"enter selects":  {k.Select, "enter"},
		"esc backs":      {k.Back, "esc"},
		"ctrl+c quits":   {k.Quit, "ctrl+c"},
		"? helps":        {k.Help, "?"},
		"/ filters":      {k.Filter, "/"},
		"j moves down":   {k.Down, "j"},
		"k moves up":     {k.Up, "k"},
		"arrow down":     {k.Down, "down"},
		"arrow up":       {k.Up, "up"},
		"space is space": {Binding{Keys: []string{"space"}}, "space"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if !tc.b.Matches(tc.key) {
				t.Fatalf("binding %+v should match %q", tc.b, tc.key)
			}
		})
	}
}

func TestBindingRejectsUnknown(t *testing.T) {
	k := DefaultKeymap()
	if k.Select.Matches("esc") {
		t.Fatal("select must not match esc")
	}
	if k.Quit.Matches("q") {
		t.Fatal("quit is ctrl+c only, must not match bare q")
	}
	if (Binding{}).Matches("enter") {
		t.Fatal("empty binding must not match")
	}
}

func TestKeymapRowsStableOrder(t *testing.T) {
	k := DefaultKeymap()
	rows := k.Rows()
	if len(rows) != 7 {
		t.Fatalf("want 7 rows, got %d", len(rows))
	}
	wantFirst := []string{"k", "j", "enter", "esc", "ctrl+c", "?", "/"}
	for i, want := range wantFirst {
		if len(rows[i].Keys) == 0 || rows[i].Keys[0] != want {
			t.Fatalf("row %d: want canonical %q, got %+v", i, want, rows[i].Keys)
		}
		if rows[i].Desc == "" {
			t.Fatalf("row %d: empty help text", i)
		}
	}
}

func TestKeyStringsAreV2(t *testing.T) {
	// Pinned v2 contract: "space" not " ".
	if got := runeKey(' ').String(); got != "space" {
		t.Fatalf("space key = %q, want %q", got, "space")
	}
	if got := specialKey(tea.KeyEnter).String(); got != "enter" {
		t.Fatalf("enter key = %q", got)
	}
}
