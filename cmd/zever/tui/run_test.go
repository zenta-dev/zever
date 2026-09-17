package tui

import (
	"context"
	"errors"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func TestExecGetters(t *testing.T) {
	m := NewExec("serve", "zever serve", nil)
	if m.Title() != "serve" || m.CLI() != "zever serve" {
		t.Fatalf("getters wrong: %+v", m)
	}
	if m.State() != ExecRunning {
		t.Fatalf("initial state = %v, want ExecRunning", m.State())
	}
	if m.Init() == nil {
		t.Fatal("Init must return spinner tick cmd")
	}
}

func TestExecStartSuccess(t *testing.T) {
	m := NewExec("seed", "zever seed", func(_ context.Context) (string, error) {
		return "seeded 3 rows", nil
	})
	msg := m.Start()
	done, ok := msg.(ExecDoneMsg)
	if !ok {
		t.Fatalf("Start = %T, want ExecDoneMsg", msg)
	}
	if done.Output != "seeded 3 rows" {
		t.Fatalf("output = %q", done.Output)
	}
}

func TestExecStartNilFn(t *testing.T) {
	m := NewExec("x", "zever x", nil)
	if _, ok := m.Start().(ExecDoneMsg); !ok {
		t.Fatal("nil fn must complete empty")
	}
}

func TestExecStartError(t *testing.T) {
	boom := errors.New("boom")
	m := NewExec("x", "zever x", func(_ context.Context) (string, error) {
		return "", boom
	})
	msg := m.Start()
	got, ok := msg.(ExecErrMsg)
	if !ok {
		t.Fatalf("Start = %T, want ExecErrMsg", msg)
	}
	if !errors.Is(got.Err, boom) {
		t.Fatalf("err = %v", got.Err)
	}
}

func TestExecDoneFlow(t *testing.T) {
	m := NewExec("serve", "zever serve", nil)
	got, cmd := m.Update(ExecDoneMsg{Output: "listening"})
	if cmd != nil {
		t.Fatal("done must yield nil cmd")
	}
	em, ok := got.(ExecModel)
	if !ok {
		t.Fatalf("Update = %T, want ExecModel", got)
	}
	if em.State() != ExecDone || em.Output() != "listening" {
		t.Fatalf("state=%v output=%q", em.State(), em.Output())
	}
	view := stripANSI(em.View().Content)
	if !containsStr(view, "equivalent CLI: zever serve") {
		t.Fatalf("result view missing CLI line:\n%s", view)
	}
	if !containsStr(view, "listening") {
		t.Fatalf("result view missing output:\n%s", view)
	}
}

func TestExecFailedFlow(t *testing.T) {
	m := NewExec("migrate", "zever migrate", nil)
	got, _ := m.Update(ExecErrMsg{Err: errors.New("dns down")})
	em, ok := got.(ExecModel)
	if !ok {
		t.Fatalf("Update = %T, want ExecModel", got)
	}
	if em.State() != ExecFailed {
		t.Fatalf("state = %v, want ExecFailed", em.State())
	}
	if em.Err() == nil || em.Err().Error() != "dns down" {
		t.Fatalf("err = %v", em.Err())
	}
	view := stripANSI(em.View().Content)
	if !containsStr(view, "equivalent CLI: zever migrate") {
		t.Fatalf("error view missing CLI line:\n%s", view)
	}
	if !containsStr(view, "dns down") {
		t.Fatalf("error view missing error:\n%s", view)
	}
}

func TestExecCanceledErrNormalizes(t *testing.T) {
	for _, err := range []error{ErrCanceled, context.Canceled} {
		m := NewExec("x", "zever x", nil)
		got, _ := m.Update(ExecErrMsg{Err: err})
		emCancel, ok := got.(ExecModel)
		if !ok {
			t.Fatalf("Update = %T, want ExecModel", got)
		}
		if emCancel.State() != ExecCanceled {
			t.Fatalf("err %v: want ExecCanceled", err)
		}
	}
}

func TestExecEscCancelsRunning(t *testing.T) {
	m := NewExec("serve", "zever serve", nil)
	got, cmd := m.Update(specialKey(tea.KeyEscape))
	em, ok := got.(ExecModel)
	if !ok {
		t.Fatalf("Update = %T, want ExecModel", got)
	}
	if em.State() != ExecCanceled {
		t.Fatalf("state = %v, want ExecCanceled", em.State())
	}
	if !errors.Is(em.Err(), ErrCanceled) {
		t.Fatalf("err = %v, want ErrCanceled", em.Err())
	}
	if cmd == nil {
		t.Fatal("cancel must emit CanceledMsg cmd")
	}
	if _, ok := cmd().(CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want CanceledMsg", cmd())
	}
	view := stripANSI(em.View().Content)
	if !containsStr(view, "canceled") {
		t.Fatalf("cancel view missing marker:\n%s", view)
	}
}

func TestExecEscAfterDoneNoop(t *testing.T) {
	m := NewExec("x", "zever x", nil)
	done, _ := m.Update(ExecDoneMsg{})
	got, cmd := done.Update(specialKey(tea.KeyEscape))
	doneModel, ok := got.(ExecModel)
	if !ok {
		t.Fatalf("Update = %T, want ExecModel", got)
	}
	if doneModel.State() != ExecDone {
		t.Fatal("esc after done must not cancel")
	}
	if cmd != nil {
		t.Fatal("esc after done must yield nil cmd")
	}
}

func TestExecSpinnerTick(t *testing.T) {
	m := NewExec("x", "zever x", nil)
	initCmd := m.Init()
	if initCmd == nil {
		t.Fatal("Init must arm spinner")
	}
	tick := initCmd()
	if _, ok := tick.(spinner.TickMsg); !ok {
		t.Fatalf("init cmd = %T, want spinner.TickMsg", tick)
	}
	got, _ := m.Update(tick)
	tickModel, ok := got.(ExecModel)
	if !ok {
		t.Fatalf("Update = %T, want ExecModel", got)
	}
	if tickModel.State() != ExecRunning {
		t.Fatal("tick must keep running state")
	}
}

func TestExecRunningView(t *testing.T) {
	m := NewExec("serve", "zever serve", nil)
	view := stripANSI(m.View().Content)
	if !containsStr(view, "serve") || !containsStr(view, "esc cancel") {
		t.Fatalf("running view wrong:\n%s", view)
	}
}

func TestExecIgnoresUnrelatedInput(t *testing.T) {
	m := NewExec("x", "zever x", nil)
	got, cmd := m.Update(runeKey('j'))
	keyModel, ok := got.(ExecModel)
	if !ok {
		t.Fatalf("Update = %T, want ExecModel", got)
	}
	if keyModel.State() != ExecRunning || cmd != nil {
		t.Fatal("unrelated key while running must be a no-op")
	}
	got, cmd = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	sizeModel, ok := got.(ExecModel)
	if !ok {
		t.Fatalf("Update = %T, want ExecModel", got)
	}
	if sizeModel.State() != ExecRunning || cmd != nil {
		t.Fatal("unknown msg must be a no-op")
	}
}
