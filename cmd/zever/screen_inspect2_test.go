package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

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
