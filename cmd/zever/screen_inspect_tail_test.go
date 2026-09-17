package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

func TestInspectRemainderBranches(t *testing.T) {
	// Compile outDir default fills ./generated at exec build time.
	cm := NewCompileScreen()
	cm.vals.outDir = ""
	if cli := cm.CLI(); strings.Contains(cli, "--out=") {
		t.Fatalf("form CLI must omit empty outDir, got %q", cli)
	}
	if exec := cm.buildExec(); !strings.Contains(exec.CLI(), "./generated") {
		t.Fatalf("exec CLI = %q", exec.CLI())
	}

	// Empty file inputs hit auto-discovery, which finds nothing under the
	// package dir, so every core reports no input.
	t.Chdir(t.TempDir())
	for name, fn := range map[string]tui.ExecFunc{
		"compile":    makeCompileExecFn(nil, "proto", t.TempDir()),
		"check":      makeCheckExecFn(nil),
		"breaking":   makeBreakingExecFn(nil, nil),
		"fmt":        makeFmtExecFn(nil, false),
		"routes":     makeRoutesExecFn(nil),
		"explain":    makeExplainExecFn("S.Op", nil),
		"boundaries": makeBoundariesExecFn(nil),
		"graph":      makeGraphExecFn(nil),
	} {
		if _, err := fn(t.Context()); !errors.Is(err, errNoInputFiles) {
			t.Fatalf("%s empty: err = %v, want errNoInputFiles", name, err)
		}
	}

	// Plain form navigation (no completion) stays in form stage.
	for name, m := range map[string]tea.Model{
		"compile":    NewCompileScreen(),
		"check":      NewCheckScreen(),
		"breaking":   NewBreakingScreen(),
		"fmt":        NewFmtScreen(),
		"doctor":     NewDoctorScreen(),
		"routes":     NewRoutesScreen(),
		"explain":    NewExplainScreen(),
		"boundaries": NewBoundariesScreen(),
		"graph":      NewGraphScreen(),
	} {
		got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		switch s := got.(type) {
		case CompileScreen:
			if s.stage != inspectStageForm {
				t.Fatalf("%s nav: stage = %v", name, s.stage)
			}
		case CheckScreen:
			if s.stage != inspectStageForm {
				t.Fatalf("%s nav: stage = %v", name, s.stage)
			}
		case BreakingScreen:
			if s.stage != inspectStageForm {
				t.Fatalf("%s nav: stage = %v", name, s.stage)
			}
		case FmtScreen:
			if s.stage != inspectStageForm {
				t.Fatalf("%s nav: stage = %v", name, s.stage)
			}
		case DoctorScreen:
			if s.stage != inspectStageForm {
				t.Fatalf("%s nav: stage = %v", name, s.stage)
			}
		case RoutesScreen:
			if s.stage != inspectStageForm {
				t.Fatalf("%s nav: stage = %v", name, s.stage)
			}
		case ExplainScreen:
			if s.stage != inspectStageForm {
				t.Fatalf("%s nav: stage = %v", name, s.stage)
			}
		case BoundariesScreen:
			if s.stage != inspectStageForm {
				t.Fatalf("%s nav: stage = %v", name, s.stage)
			}
		case GraphScreen:
			if s.stage != inspectStageForm {
				t.Fatalf("%s nav: stage = %v", name, s.stage)
			}
		default:
			t.Fatalf("%s nav: unexpected %T", name, got)
		}
	}

	// esc while running forwards to the exec (cancel path) on every screen.
	submit := func(m tea.Model) tea.Model {
		switch s := m.(type) {
		case CompileScreen:
			s.form.State = huh.StateCompleted
			got, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			return got
		case CheckScreen:
			s.form.State = huh.StateCompleted
			got, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			return got
		case BreakingScreen:
			s.form.State = huh.StateCompleted
			got, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			return got
		case FmtScreen:
			s.form.State = huh.StateCompleted
			got, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			return got
		case DoctorScreen:
			s.form.State = huh.StateCompleted
			got, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			return got
		case RoutesScreen:
			s.form.State = huh.StateCompleted
			got, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			return got
		case ExplainScreen:
			s.form.State = huh.StateCompleted
			got, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			return got
		case BoundariesScreen:
			s.form.State = huh.StateCompleted
			got, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			return got
		case GraphScreen:
			s.form.State = huh.StateCompleted
			got, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			return got
		}
		t.Fatalf("unknown %T", m)
		return m
	}
	for name, m := range map[string]tea.Model{
		"compile":    NewCompileScreen(),
		"check":      NewCheckScreen(),
		"breaking":   NewBreakingScreen(),
		"fmt":        NewFmtScreen(),
		"doctor":     NewDoctorScreen(),
		"routes":     NewRoutesScreen(),
		"explain":    NewExplainScreen(),
		"boundaries": NewBoundariesScreen(),
		"graph":      NewGraphScreen(),
	} {
		got, cmd := submit(m).Update(inspectKeyPress("esc"))
		_ = got
		if cmd == nil {
			t.Fatalf("%s running esc: want cancel cmd", name)
		}
		if _, ok := cmd().(tui.CanceledMsg); !ok {
			t.Fatalf("%s running esc: cmd = %T", name, cmd())
		}
	}

	// Graph exec error stays in exec stage; esc then goes back.
	gs := NewGraphScreen()
	gs.form.State = huh.StateCompleted
	got, _ := gs.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	gs, ok := got.(GraphScreen)
	if !ok {
		t.Fatalf("Update = %T, want GraphScreen", got)
	}
	got, _ = gs.Update(tui.ExecErrMsg{Err: context.DeadlineExceeded})
	gs, ok = got.(GraphScreen)
	if !ok {
		t.Fatalf("Update = %T, want GraphScreen", got)
	}
	if gs.stage != inspectStageExec {
		t.Fatalf("graph err stage = %v", gs.stage)
	}
	_, cmd := gs.Update(inspectKeyPress("esc"))
	mustBackMsg(t, cmd)

	// Log-stage scroll + LogBackMsg paths.
	gl := NewGraphScreen()
	gl.form.State = huh.StateCompleted
	got, _ = gl.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	gl, ok = got.(GraphScreen)
	if !ok {
		t.Fatalf("Update = %T, want GraphScreen", got)
	}
	got, _ = gl.Update(tui.ExecDoneMsg{Output: "a\nb"})
	gl, ok = got.(GraphScreen)
	if !ok {
		t.Fatalf("Update = %T, want GraphScreen", got)
	}
	if _, keyCmd := gl.Update(inspectKeyPress("j")); keyCmd == nil {
		_ = cmd
	}
	_, cmd = gl.Update(tui.LogBackMsg{})
	mustBackMsg(t, cmd)
}
