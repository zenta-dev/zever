package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

func inspectKeyPress(s string) tea.Msg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		r := []rune(s)[0]
		return tea.KeyPressMsg{Code: r, Text: s}
	}
}

func TestSplitFileList(t *testing.T) {
	got := splitFileList("a.zen, b.zen  c.zen\nd.zen;e.zen")
	want := []string{"a.zen", "b.zen", "c.zen", "d.zen", "e.zen"}
	if len(got) != len(want) {
		t.Fatalf("split = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("split = %v, want %v", got, want)
		}
	}
	if len(splitFileList("")) != 0 {
		t.Fatal("empty input must yield no files")
	}
}

func TestRegisterInspectScreens(t *testing.T) {
	entries := RegisterInspectScreens()
	if len(entries) != 10 {
		t.Fatalf("entries = %d, want 10", len(entries))
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Group != "Inspect" {
			t.Fatalf("entry %q group = %q, want Inspect", e.Name, e.Group)
		}
		if e.Name == "" || e.Desc == "" || e.CLI == "" || e.Screen == "" {
			t.Fatalf("entry %+v has empty field", e)
		}
		seen[e.Name] = true
	}
	for _, want := range []string{"compile", "check", "breaking", "fmt", "doctor", "routes", "explain", "check-boundaries", "graph"} {
		if !seen[want] {
			t.Fatalf("missing entry %q", want)
		}
	}
}

func TestCompileScreenInitEscSubmit(t *testing.T) {
	m := NewCompileScreen()
	if m.Init() == nil {
		t.Fatal("Init must arm form")
	}
	v := m.View().Content
	if !strings.Contains(v, "compile") || !strings.Contains(v, "zever compile") {
		t.Fatalf("form view missing title/CLI preview:\n%s", v)
	}
	got, cmd := m.Update(inspectKeyPress("esc"))
	_ = got
	if cmd == nil {
		t.Fatal("esc must yield back cmd")
	}
	if _, ok := cmd().(tui.BackMsg); !ok {
		t.Fatalf("esc cmd = %T, want tui.BackMsg", cmd())
	}
	// Force completion to exercise submit path without driving huh keys.
	m2 := NewCompileScreen()
	m2.form.State = huh.StateCompleted
	got2, cmd2 := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cs, ok := got2.(CompileScreen)
	if !ok {
		t.Fatalf("Update = %T, want CompileScreen", got2)
	}
	if cs.stage != inspectStageExec {
		t.Fatalf("after completed form stage = %v, want exec", cs.stage)
	}
	if cmd2 == nil {
		t.Fatal("submit must yield exec cmds")
	}
	if !strings.Contains(cs.CLI(), "zever compile") {
		t.Fatalf("CLI = %q", cs.CLI())
	}
}

func TestCheckScreenGoldenAndEsc(t *testing.T) {
	m := NewCheckScreen()
	v := m.View().Content
	if !strings.Contains(v, "check") || !strings.Contains(v, "zever check") {
		t.Fatalf("check view:\n%s", v)
	}
	_, cmd := m.Update(inspectKeyPress("esc"))
	if cmd == nil || cmd() == nil {
		t.Fatal("esc must yield msg")
	}
	m2 := NewCheckScreen()
	m2.form.State = huh.StateCompleted
	got, _ := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	checkScreen, ok := got.(CheckScreen)
	if !ok {
		t.Fatalf("Update = %T, want CheckScreen", got)
	}
	if checkScreen.stage != inspectStageExec {
		t.Fatal("check must transition to exec on submit")
	}
}

func TestBreakingScreenSubmit(t *testing.T) {
	m := NewBreakingScreen()
	if !strings.Contains(m.View().Content, "breaking") {
		t.Fatalf("breaking view:\n%s", m.View().Content)
	}
	_, cmd := m.Update(inspectKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc must yield back cmd")
	}
	m2 := NewBreakingScreen()
	m2.form.State = huh.StateCompleted
	got, _ := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	bs, ok := got.(BreakingScreen)
	if !ok {
		t.Fatalf("Update = %T, want BreakingScreen", got)
	}
	if bs.stage != inspectStageExec {
		t.Fatal("breaking must transition to exec")
	}
	if !strings.Contains(bs.CLI(), "zever breaking") || !strings.Contains(bs.CLI(), "--") {
		t.Fatalf("CLI = %q", bs.CLI())
	}
}

func TestFmtScreenSubmit(t *testing.T) {
	m := NewFmtScreen()
	if !strings.Contains(m.View().Content, "zever fmt") {
		t.Fatalf("fmt view:\n%s", m.View().Content)
	}
	_, cmd := m.Update(inspectKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc must yield back cmd")
	}
	m2 := NewFmtScreen()
	m2.form.State = huh.StateCompleted
	got, _ := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	fmtScreen, ok := got.(FmtScreen)
	if !ok {
		t.Fatalf("Update = %T, want FmtScreen", got)
	}
	if fmtScreen.stage != inspectStageExec {
		t.Fatal("fmt must transition to exec")
	}
}

func TestCompileScreenTeatest(t *testing.T) {
	m := NewCompileScreen()
	tm := teatest.NewTestModel(t, m)
	tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	tm.WaitFinished(t)
	if _, ok := tm.FinalModel(t).(CompileScreen); !ok {
		t.Fatalf("final = %T, want CompileScreen", tm.FinalModel(t))
	}
}
