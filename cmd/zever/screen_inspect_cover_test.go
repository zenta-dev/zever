package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

const inspectScreenUserSchema = `entity User {
  id: uuid @primary
  email: string @unique
}

service UserService {
  rpc GetUser(id: uuid) -> User {
    http: GET "/v1/users/{id}"
    auth: required
  }
}
`

func writeInspectScreenFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func mustBackMsg(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("want back cmd, got nil")
	}
	if _, ok := cmd().(tui.BackMsg); !ok {
		t.Fatalf("cmd = %T, want tui.BackMsg", cmd())
	}
}

func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestInspectCtxErr(t *testing.T) {
	if err := inspectCtxErr(context.Background()); err != nil {
		t.Fatalf("background ctx: %v", err)
	}
	if err := inspectCtxErr(canceledCtx()); err == nil {
		t.Fatal("canceled ctx must yield error")
	}
}

func TestValidateExplainOp(t *testing.T) {
	if err := validateExplainOp(""); err == nil {
		t.Fatal("empty op must fail")
	}
	if err := validateExplainOp("UserService.GetUser"); err != nil {
		t.Fatalf("valid op: %v", err)
	}
}

func TestAllScreensInit(t *testing.T) {
	screens := []struct {
		name string
		init tea.Cmd
	}{
		{"compile", NewCompileScreen().Init()},
		{"check", NewCheckScreen().Init()},
		{"breaking", NewBreakingScreen().Init()},
		{"fmt", NewFmtScreen().Init()},
		{"doctor", NewDoctorScreen().Init()},
		{"routes", NewRoutesScreen().Init()},
		{"explain", NewExplainScreen().Init()},
		{"boundaries", NewBoundariesScreen().Init()},
		{"graph", NewGraphScreen().Init()},
	}
	for _, s := range screens {
		if s.init == nil {
			t.Fatalf("%s Init nil", s.name)
		}
	}
}

func TestAbortedFormsGoBack(t *testing.T) {
	m := NewCompileScreen()
	m.form.State = huh.StateAborted
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mustBackMsg(t, cmd)

	m2 := NewCheckScreen()
	m2.form.State = huh.StateAborted
	_, cmd = m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mustBackMsg(t, cmd)

	m3 := NewBreakingScreen()
	m3.form.State = huh.StateAborted
	_, cmd = m3.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mustBackMsg(t, cmd)

	m4 := NewFmtScreen()
	m4.form.State = huh.StateAborted
	_, cmd = m4.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mustBackMsg(t, cmd)

	m5 := NewDoctorScreen()
	m5.form.State = huh.StateAborted
	_, cmd = m5.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mustBackMsg(t, cmd)

	m6 := NewRoutesScreen()
	m6.form.State = huh.StateAborted
	_, cmd = m6.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mustBackMsg(t, cmd)

	m7 := NewExplainScreen()
	m7.form.State = huh.StateAborted
	_, cmd = m7.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mustBackMsg(t, cmd)

	m8 := NewBoundariesScreen()
	m8.form.State = huh.StateAborted
	_, cmd = m8.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mustBackMsg(t, cmd)

	m9 := NewGraphScreen()
	m9.form.State = huh.StateAborted
	_, cmd = m9.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mustBackMsg(t, cmd)
}

func TestExecStageDoneEscBack(t *testing.T) {
	// Compile: submit → running view → done → esc back.
	cs := NewCompileScreen()
	cs.form.State = huh.StateCompleted
	got, _ := cs.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cs, ok := got.(CompileScreen)
	if !ok {
		t.Fatalf("Update = %T, want CompileScreen", got)
	}
	if !strings.Contains(cs.View().Content, "compile") {
		t.Fatalf("running view:\n%s", cs.View().Content)
	}
	done, _ := cs.Update(tui.ExecDoneMsg{Output: "ok"})
	csd, ok := done.(CompileScreen)
	if !ok {
		t.Fatalf("Update = %T, want CompileScreen", done)
	}
	if !strings.Contains(csd.View().Content, "equivalent CLI:") {
		t.Fatalf("done view:\n%s", csd.View().Content)
	}
	_, cmd := csd.Update(inspectKeyPress("esc"))
	mustBackMsg(t, cmd)

	// Check failed flow shows error + CLI, then esc back.
	ks := NewCheckScreen()
	ks.form.State = huh.StateCompleted
	got, _ = ks.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ks, ok = got.(CheckScreen)
	if !ok {
		t.Fatalf("Update = %T, want CheckScreen", got)
	}
	failed, _ := ks.Update(tui.ExecErrMsg{Err: context.DeadlineExceeded})
	ksf, ok := failed.(CheckScreen)
	if !ok {
		t.Fatalf("Update = %T, want CheckScreen", failed)
	}
	if !strings.Contains(ksf.View().Content, "zever check") {
		t.Fatalf("failed view:\n%s", ksf.View().Content)
	}
	_, cmd = ksf.Update(inspectKeyPress("esc"))
	mustBackMsg(t, cmd)

	// Breaking + fmt done views carry their CLI lines.
	bs := NewBreakingScreen()
	bs.form.State = huh.StateCompleted
	got, _ = bs.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	bs, ok = got.(BreakingScreen)
	if !ok {
		t.Fatalf("Update = %T, want BreakingScreen", got)
	}
	bdone, _ := bs.Update(tui.ExecDoneMsg{Output: "ok"})
	bdoneScreen, ok := bdone.(BreakingScreen)
	if !ok {
		t.Fatalf("Update = %T, want BreakingScreen", bdone)
	}
	if !strings.Contains(bdoneScreen.View().Content, "zever breaking") {
		t.Fatalf("breaking done view:\n%s", bdoneScreen.View().Content)
	}
	_, cmd = bdoneScreen.Update(inspectKeyPress("esc"))
	mustBackMsg(t, cmd)

	fs := NewFmtScreen()
	fs.form.State = huh.StateCompleted
	got, _ = fs.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	fs, ok = got.(FmtScreen)
	if !ok {
		t.Fatalf("Update = %T, want FmtScreen", got)
	}
	fdone, _ := fs.Update(tui.ExecDoneMsg{Output: "ok"})
	fdoneScreen, ok := fdone.(FmtScreen)
	if !ok {
		t.Fatalf("Update = %T, want FmtScreen", fdone)
	}
	if !strings.Contains(fdoneScreen.View().Content, "zever fmt") {
		t.Fatalf("fmt done view:\n%s", fdoneScreen.View().Content)
	}
	_, cmd = fdoneScreen.Update(inspectKeyPress("esc"))
	mustBackMsg(t, cmd)

	// Doctor / routes / explain / boundaries done + esc-back.
	ds := NewDoctorScreen()
	ds.form.State = huh.StateCompleted
	got, _ = ds.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ds, ok = got.(DoctorScreen)
	if !ok {
		t.Fatalf("Update = %T, want DoctorScreen", got)
	}
	ddone, _ := ds.Update(tui.ExecDoneMsg{Output: "ok"})
	ddoneScreen, ok := ddone.(DoctorScreen)
	if !ok {
		t.Fatalf("Update = %T, want DoctorScreen", ddone)
	}
	if !strings.Contains(ddoneScreen.View().Content, "zever doctor") {
		t.Fatalf("doctor done view:\n%s", ddoneScreen.View().Content)
	}
	_, cmd = ddoneScreen.Update(inspectKeyPress("esc"))
	mustBackMsg(t, cmd)

	rs := NewRoutesScreen()
	rs.form.State = huh.StateCompleted
	got, _ = rs.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rs, ok = got.(RoutesScreen)
	if !ok {
		t.Fatalf("Update = %T, want RoutesScreen", got)
	}
	rdone, _ := rs.Update(tui.ExecDoneMsg{Output: "ok"})
	rdoneScreen, ok := rdone.(RoutesScreen)
	if !ok {
		t.Fatalf("Update = %T, want RoutesScreen", rdone)
	}
	if !strings.Contains(rdoneScreen.View().Content, "zever routes") {
		t.Fatalf("routes done view:\n%s", rdoneScreen.View().Content)
	}
	_, cmd = rdoneScreen.Update(inspectKeyPress("esc"))
	mustBackMsg(t, cmd)

	es := NewExplainScreen()
	es.form.State = huh.StateCompleted
	got, _ = es.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	es, ok = got.(ExplainScreen)
	if !ok {
		t.Fatalf("Update = %T, want ExplainScreen", got)
	}
	edone, _ := es.Update(tui.ExecDoneMsg{Output: "ok"})
	edoneScreen, ok := edone.(ExplainScreen)
	if !ok {
		t.Fatalf("Update = %T, want ExplainScreen", edone)
	}
	if !strings.Contains(edoneScreen.View().Content, "zever explain") {
		t.Fatalf("explain done view:\n%s", edoneScreen.View().Content)
	}
	_, cmd = edoneScreen.Update(inspectKeyPress("esc"))
	mustBackMsg(t, cmd)

	ns := NewBoundariesScreen()
	ns.form.State = huh.StateCompleted
	got, _ = ns.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ns, ok = got.(BoundariesScreen)
	if !ok {
		t.Fatalf("Update = %T, want BoundariesScreen", got)
	}
	ndone, _ := ns.Update(tui.ExecDoneMsg{Output: "ok"})
	ndoneScreen, ok := ndone.(BoundariesScreen)
	if !ok {
		t.Fatalf("Update = %T, want BoundariesScreen", ndone)
	}
	if !strings.Contains(ndoneScreen.View().Content, "check-boundaries") {
		t.Fatalf("boundaries done view:\n%s", ndoneScreen.View().Content)
	}
	_, cmd = ndoneScreen.Update(inspectKeyPress("esc"))
	mustBackMsg(t, cmd)

	// Graph: exec view before done, log view after, esc back from log.
	gs := NewGraphScreen()
	gs.form.State = huh.StateCompleted
	got, _ = gs.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	gs, ok = got.(GraphScreen)
	if !ok {
		t.Fatalf("Update = %T, want GraphScreen", got)
	}
	if !strings.Contains(gs.View().Content, "graph") {
		t.Fatalf("graph exec view:\n%s", gs.View().Content)
	}
	gdone, _ := gs.Update(tui.ExecDoneMsg{Output: "line1"})
	gl, ok := gdone.(GraphScreen)
	if !ok {
		t.Fatalf("Update = %T, want GraphScreen", gdone)
	}
	if gl.stage != inspectStageLog {
		t.Fatalf("graph stage = %v, want log", gl.stage)
	}
	_, cmd = gl.Update(inspectKeyPress("esc"))
	mustBackMsg(t, cmd)
	// Window resize in log stage keeps the pager alive.
	_, _ = gl.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
}

func TestExecStageRunningEscCancels(t *testing.T) {
	cs := NewCheckScreen()
	cs.form.State = huh.StateCompleted
	got, _ := cs.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cs, ok := got.(CheckScreen)
	if !ok {
		t.Fatalf("Update = %T, want CheckScreen", got)
	}
	got, cmd := cs.Update(inspectKeyPress("esc"))
	esc, ok := got.(CheckScreen)
	if !ok {
		t.Fatalf("Update = %T, want CheckScreen", got)
	}
	if esc.exec.State() != tui.ExecCanceled {
		t.Fatalf("state = %v, want ExecCanceled", esc.exec.State())
	}
	if cmd == nil {
		t.Fatal("cancel must emit cmd")
	}
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want CanceledMsg", cmd())
	}
}

func TestMakeExecFnsRunCores(t *testing.T) {
	dir := t.TempDir()
	schema := writeInspectScreenFixture(t, dir, "user.zen", inspectScreenUserSchema)
	ctx := context.Background()

	if out, err := makeCompileExecFn([]string{schema}, "proto", filepath.Join(dir, "out"))(ctx); err != nil {
		t.Fatalf("compile: %v", err)
	} else if !strings.Contains(out, "compiled") {
		t.Fatalf("compile output = %q", out)
	}

	if out, err := makeCheckExecFn([]string{schema})(ctx); err != nil {
		t.Fatalf("check: %v", err)
	} else if !strings.Contains(out, "valid") {
		t.Fatalf("check output = %q", out)
	}

	if out, err := makeBreakingExecFn([]string{schema}, []string{schema})(ctx); err != nil {
		t.Fatalf("breaking identical: %v", err)
	} else if !strings.Contains(out, "no differences") {
		t.Fatalf("breaking output = %q", out)
	}

	if _, err := makeFmtExecFn([]string{schema}, true)(ctx); err != nil {
		t.Fatalf("fmt write: %v", err)
	}

	if out, err := makeDoctorExecFn("")(ctx); err != nil {
		t.Fatalf("doctor: %v", err)
	} else if !strings.Contains(out, "config") {
		t.Fatalf("doctor output missing config row:\n%s", out)
	}

	if out, err := makeRoutesExecFn([]string{schema})(ctx); err != nil {
		t.Fatalf("routes: %v", err)
	} else if !strings.Contains(out, "GET") {
		t.Fatalf("routes output = %q", out)
	}

	if out, err := makeExplainExecFn("UserService.GetUser", []string{schema})(ctx); err != nil {
		t.Fatalf("explain: %v", err)
	} else if !strings.Contains(out, "transports") {
		t.Fatalf("explain output = %q", out)
	}

	if out, err := makeBoundariesExecFn([]string{schema})(ctx); err != nil {
		t.Fatalf("boundaries: %v", err)
	} else if !strings.Contains(out, "no cross-module") {
		t.Fatalf("boundaries output = %q", out)
	}

	if out, err := makeGraphExecFn([]string{schema})(ctx); err != nil {
		t.Fatalf("graph: %v", err)
	} else if !strings.Contains(out, "mermaid") {
		t.Fatalf("graph output = %q", out)
	}
}

func TestMakeExecFnsCanceled(t *testing.T) {
	ctx := canceledCtx()
	fns := map[string]tui.ExecFunc{
		"compile":    makeCompileExecFn(nil, "", ""),
		"check":      makeCheckExecFn(nil),
		"breaking":   makeBreakingExecFn(nil, nil),
		"fmt":        makeFmtExecFn(nil, false),
		"doctor":     makeDoctorExecFn(""),
		"routes":     makeRoutesExecFn(nil),
		"explain":    makeExplainExecFn("S.Op", nil),
		"boundaries": makeBoundariesExecFn(nil),
		"graph":      makeGraphExecFn(nil),
	}
	for name, fn := range fns {
		if _, err := fn(ctx); err == nil {
			t.Fatalf("%s canceled: want error", name)
		}
	}
}
