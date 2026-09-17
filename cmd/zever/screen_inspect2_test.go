package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

func TestDoctorScreenSubmit(t *testing.T) {
	m := NewDoctorScreen()
	if !strings.Contains(m.View().Content, "doctor") {
		t.Fatalf("doctor view:\n%s", m.View().Content)
	}
	_, cmd := m.Update(inspectKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc must yield back cmd")
	}
	m2 := NewDoctorScreen()
	m2.form.State = huh.StateCompleted
	got, _ := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ds, ok := got.(DoctorScreen)
	if !ok {
		t.Fatalf("Update = %T, want DoctorScreen", got)
	}
	if ds.stage != inspectStageExec {
		t.Fatal("doctor must transition to exec")
	}
	if !strings.Contains(ds.CLI(), "zever doctor") {
		t.Fatalf("CLI = %q", ds.CLI())
	}
}

func TestConfigScreenSubmit(t *testing.T) {
	m := NewConfigScreen()
	if !strings.Contains(m.View().Content, "config show") {
		t.Fatalf("config view:\n%s", m.View().Content)
	}
	_, cmd := m.Update(inspectKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc must yield back cmd")
	}
	m2 := NewConfigScreen()
	m2.form.State = huh.StateCompleted
	got, _ := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cs, ok := got.(ConfigScreen)
	if !ok {
		t.Fatalf("Update = %T, want ConfigScreen", got)
	}
	if cs.stage != inspectStageExec {
		t.Fatal("config must transition to exec")
	}
	if !strings.Contains(cs.CLI(), "zever config show") {
		t.Fatalf("CLI = %q", cs.CLI())
	}
}

// TestConfigScreenNormalFlow submits the (empty) form, then simulates the
// exec's ExecDoneMsg the same way TestGraphScreenSubmitAndLog simulates
// graph's exec completion: deterministic, no goroutine timing, no real
// config.Default() I/O needed to prove the state machine and view wiring.
func TestConfigScreenNormalFlow(t *testing.T) {
	m := NewConfigScreen()
	m.form.State = huh.StateCompleted
	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cs, ok := got.(ConfigScreen)
	if !ok {
		t.Fatalf("Update = %T, want ConfigScreen", got)
	}
	if cs.stage != inspectStageExec {
		t.Fatal("config must transition to exec")
	}
	done, _ := cs.Update(tui.ExecDoneMsg{Output: "ai  adapter=anthropic"})
	cs2, ok := done.(ConfigScreen)
	if !ok {
		t.Fatalf("Update = %T, want ConfigScreen", done)
	}
	if cs2.exec.State() != tui.ExecDone {
		t.Fatalf("state = %v, want ExecDone", cs2.exec.State())
	}
	if !strings.Contains(cs2.View().Content, "adapter=") {
		t.Fatalf("done view missing config rows:\n%s", cs2.View().Content)
	}
}

// TestConfigScreenErrorFlow points the config path at a file that cannot be
// loaded and runs the real exec func synchronously (deterministic: no
// network, a plain missing-file stat), then feeds its ExecErrMsg through
// Update to drive the screen to ExecFailed instead of ExecDone.
func TestConfigScreenErrorFlow(t *testing.T) {
	m := NewConfigScreen()
	m.vals.configPath = "nope.yaml"
	m.form.State = huh.StateCompleted
	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cs, ok := got.(ConfigScreen)
	if !ok {
		t.Fatalf("Update = %T, want ConfigScreen", got)
	}
	if cs.stage != inspectStageExec {
		t.Fatal("config must transition to exec")
	}

	_, err := makeConfigExecFn("nope.yaml")(t.Context())
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}

	failed, _ := cs.Update(tui.ExecErrMsg{Err: err})
	cs2, ok := failed.(ConfigScreen)
	if !ok {
		t.Fatalf("Update = %T, want ConfigScreen", failed)
	}
	if cs2.exec.State() != tui.ExecFailed {
		t.Fatalf("state = %v, want ExecFailed", cs2.exec.State())
	}
	if !strings.Contains(cs2.View().Content, "load config") {
		t.Fatalf("error view missing load failure:\n%s", cs2.View().Content)
	}
}

// TestConfigScreenCancel asserts esc while the exec is running cancels it
// (ExecCanceled) and emits tui.CanceledMsg, the same esc-back contract every
// other Inspect screen uses.
func TestConfigScreenCancel(t *testing.T) {
	cs := NewConfigScreen()
	cs.form.State = huh.StateCompleted
	got, _ := cs.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	running, ok := got.(ConfigScreen)
	if !ok {
		t.Fatalf("Update = %T, want ConfigScreen", got)
	}
	if running.exec.State() != tui.ExecRunning {
		t.Fatalf("state = %v, want ExecRunning", running.exec.State())
	}
	got2, cmd := running.Update(inspectKeyPress("esc"))
	canceled, ok := got2.(ConfigScreen)
	if !ok {
		t.Fatalf("Update = %T, want ConfigScreen", got2)
	}
	if canceled.exec.State() != tui.ExecCanceled {
		t.Fatalf("state = %v, want ExecCanceled", canceled.exec.State())
	}
	if cmd == nil {
		t.Fatal("cancel must emit cmd")
	}
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want CanceledMsg", cmd())
	}
}

func TestConfigScreenTeatest(t *testing.T) {
	m := NewConfigScreen()
	tm := teatest.NewTestModel(t, m)
	tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	tm.WaitFinished(t)
	if _, ok := tm.FinalModel(t).(ConfigScreen); !ok {
		t.Fatalf("final = %T, want ConfigScreen", tm.FinalModel(t))
	}
}

func TestRoutesScreenSubmit(t *testing.T) {
	m := NewRoutesScreen()
	if !strings.Contains(m.View().Content, "zever routes") {
		t.Fatalf("routes view:\n%s", m.View().Content)
	}
	_, cmd := m.Update(inspectKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc must yield back cmd")
	}
	m2 := NewRoutesScreen()
	m2.form.State = huh.StateCompleted
	got, _ := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	routesScreen, ok := got.(RoutesScreen)
	if !ok {
		t.Fatalf("Update = %T, want RoutesScreen", got)
	}
	if routesScreen.stage != inspectStageExec {
		t.Fatal("routes must transition to exec")
	}
}

func TestExplainScreenSubmit(t *testing.T) {
	m := NewExplainScreen()
	if !strings.Contains(m.View().Content, "zever explain") {
		t.Fatalf("explain view:\n%s", m.View().Content)
	}
	_, cmd := m.Update(inspectKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc must yield back cmd")
	}
	m2 := NewExplainScreen()
	m2.form.State = huh.StateCompleted
	got, _ := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	es, ok := got.(ExplainScreen)
	if !ok {
		t.Fatalf("Update = %T, want ExplainScreen", got)
	}
	if es.stage != inspectStageExec {
		t.Fatal("explain must transition to exec")
	}
	if !strings.Contains(es.CLI(), "zever explain") {
		t.Fatalf("CLI = %q", es.CLI())
	}
}

func TestBoundariesScreenSubmit(t *testing.T) {
	m := NewBoundariesScreen()
	if !strings.Contains(m.View().Content, "check-boundaries") {
		t.Fatalf("boundaries view:\n%s", m.View().Content)
	}
	_, cmd := m.Update(inspectKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc must yield back cmd")
	}
	m2 := NewBoundariesScreen()
	m2.form.State = huh.StateCompleted
	got, _ := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	boundariesScreen, ok := got.(BoundariesScreen)
	if !ok {
		t.Fatalf("Update = %T, want BoundariesScreen", got)
	}
	if boundariesScreen.stage != inspectStageExec {
		t.Fatal("boundaries must transition to exec")
	}
}

func TestGraphScreenSubmitAndLog(t *testing.T) {
	m := NewGraphScreen()
	if !strings.Contains(m.View().Content, "zever graph") {
		t.Fatalf("graph view:\n%s", m.View().Content)
	}
	_, cmd := m.Update(inspectKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc must yield back cmd")
	}
	m2 := NewGraphScreen()
	m2.form.State = huh.StateCompleted
	got, _ := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	gs, ok := got.(GraphScreen)
	if !ok {
		t.Fatalf("Update = %T, want GraphScreen", got)
	}
	if gs.stage != inspectStageExec {
		t.Fatal("graph must transition to exec")
	}
	// Simulate exec completion: output must land in the pager log.
	got3, _ := gs.Update(tui.ExecDoneMsg{Output: "```mermaid\ngraph TD"})
	gs3, ok := got3.(GraphScreen)
	if !ok {
		t.Fatalf("Update = %T, want GraphScreen", got3)
	}
	if gs3.stage != inspectStageLog {
		t.Fatalf("graph stage = %v, want log", gs3.stage)
	}
	if !strings.Contains(gs3.View().Content, "mermaid") {
		t.Fatalf("log view missing graph:\n%s", gs3.View().Content)
	}
}

func TestBuildCLIs(t *testing.T) {
	if !strings.Contains(buildCheckCLI([]string{"a.zen"}), "zever check a.zen") {
		t.Fatalf("check cli: %q", buildCheckCLI([]string{"a.zen"}))
	}
	if got := buildCheckCLI(nil); got != "zever check" {
		t.Fatalf("check cli empty = %q", got)
	}
	if got := buildFmtCLI([]string{"a.zen"}, true); !strings.Contains(got, "--write") {
		t.Fatalf("fmt cli = %q", got)
	}
	if got := buildDoctorCLI(""); got != "zever doctor" {
		t.Fatalf("doctor cli = %q", got)
	}
	if got := buildDoctorCLI("z.yaml"); !strings.Contains(got, "--config") {
		t.Fatalf("doctor cli = %q", got)
	}
	if got := buildConfigCLI(""); got != "zever config show" {
		t.Fatalf("config cli = %q", got)
	}
	if got := buildConfigCLI("z.yaml"); !strings.Contains(got, "--config") {
		t.Fatalf("config cli = %q", got)
	}
	if got := buildBreakingCLI([]string{"o.zen"}, []string{"n.zen"}); !strings.Contains(got, "--") {
		t.Fatalf("breaking cli = %q", got)
	}
	if got := buildExplainCLI("S.Op", []string{"a.zen"}); !strings.Contains(got, "S.Op") {
		t.Fatalf("explain cli = %q", got)
	}
	if got := buildCompileCLI([]string{"a.zen"}, "proto", "./generated"); !strings.Contains(got, "--backend=proto") {
		t.Fatalf("compile cli = %q", got)
	}
	if got := buildGraphCLI(nil); got != "zever graph" {
		t.Fatalf("graph cli = %q", got)
	}
	if got := buildRoutesCLI(nil); got != "zever routes" {
		t.Fatalf("routes cli = %q", got)
	}
	if got := buildBoundariesCLI(nil); got != "zever check-boundaries" {
		t.Fatalf("boundaries cli = %q", got)
	}
}
