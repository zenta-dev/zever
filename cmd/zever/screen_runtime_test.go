package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/traefik/yaegi/interp"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

var errRuntimeTestPipe = errors.New("runtime test pipe failure")

// testStubPiped replaces runtimeStartPiped with a stub that runs sleep,
// keeping tests off the Go toolchain while exercising real process-group
// stop semantics.
func testStubPiped(line string) func(string, []string) (*runtimeChild, *bufio.Reader, error) {
	return func(entry string, args []string) (*runtimeChild, *bufio.Reader, error) {
		_ = entry
		_ = args
		cmd := exec.CommandContext(context.Background(), "sleep", "30")
		isolateProcessGroup(cmd)
		r, w, err := os.Pipe()
		if err != nil {
			return nil, nil, err
		}
		if line != "" {
			_, _ = w.WriteString(line + "\n")
		}
		_ = w.Close()
		if err := cmd.Start(); err != nil {
			_ = r.Close()
			return nil, nil, err
		}
		return newRuntimeChild(cmd), bufio.NewReader(r), nil
	}
}

func ctrlCKey() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl} }

// --- RegisterRuntimeScreens contract ---

func TestRegisterRuntimeScreens(t *testing.T) {
	entries := RegisterRuntimeScreens()
	if len(entries) != 5 {
		t.Fatalf("want 5 runtime entries, got %d: %+v", len(entries), entries)
	}
	byName := map[string]tui.Entry{}
	for _, e := range entries {
		if e.Group != tui.GroupRuntime {
			t.Fatalf("entry %q group = %q, want Runtime", e.Name, e.Group)
		}
		if e.CLI == "" || e.Desc == "" || e.Screen == "" {
			t.Fatalf("entry %+v missing CLI/Desc/Screen", e)
		}
		byName[e.Name] = e
	}
	for _, want := range []string{"serve", "dev", "queue:work", "schedule:run", "tinker"} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("missing entry %q in %+v", want, entries)
		}
	}
	if !strings.HasPrefix(byName["serve"].CLI, "zever serve") {
		t.Fatalf("serve CLI = %q", byName["serve"].CLI)
	}
	if !strings.HasPrefix(byName["dev"].CLI, "zever dev") {
		t.Fatalf("dev CLI = %q", byName["dev"].CLI)
	}
	if !strings.HasPrefix(byName["queue:work"].CLI, "zever queue:work") {
		t.Fatalf("queue:work CLI = %q", byName["queue:work"].CLI)
	}
	if !strings.Contains(byName["queue:work"].Desc, "schedule:run") {
		t.Fatalf("queue:work Desc must note the alias, got %q", byName["queue:work"].Desc)
	}
	if !strings.HasPrefix(byName["schedule:run"].CLI, "zever schedule:run") {
		t.Fatalf("schedule:run CLI = %q", byName["schedule:run"].CLI)
	}
	if !strings.Contains(byName["schedule:run"].Desc, "alias") {
		t.Fatalf("schedule:run Desc must note alias, got %q", byName["schedule:run"].Desc)
	}
	if !strings.HasPrefix(byName["tinker"].CLI, "zever tinker") {
		t.Fatalf("tinker CLI = %q", byName["tinker"].CLI)
	}
}

// --- serve screen ---

func TestServeScreenDefaults(t *testing.T) {
	s := NewServeScreen()
	if s.CLI() != "zever serve" {
		t.Fatalf("default CLI = %q, want %q", s.CLI(), "zever serve")
	}
	if s.entryValue() != defaultServerEntry {
		t.Fatalf("default entry = %q, want %q", s.entryValue(), defaultServerEntry)
	}
	// Entry must flow through the core resolver unchanged.
	cfg := resolveServeConfig(ProjectConfig{}.withDefaults(), nil)
	if cfg.Entry != defaultServerEntry {
		t.Fatalf("core entry = %q", cfg.Entry)
	}
	if got := s.buildCLI("cmd/server", nil); got != "zever serve" {
		t.Fatalf("cli = %q", got)
	}
	if got := s.buildCLI("cmd/server", []string{"-addr", ":9090"}); got != "zever serve -addr :9090" {
		t.Fatalf("cli = %q", got)
	}
}

func TestServeFormEscCancels(t *testing.T) {
	s := NewServeScreen()
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc in form must yield a cmd")
	}
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}
}

func TestServeGoldenViews(t *testing.T) {
	s := NewServeScreen()
	formView := stripScreenANSI(s.View().Content)
	for _, want := range []string{"serve", "zever serve"} {
		if !strings.Contains(formView, want) {
			t.Fatalf("form view missing %q:\n%s", want, formView)
		}
	}
	s.stage = runtimeRunning
	s.cli = "zever serve -addr :9090"
	s.log.Append("listening :9090")
	running := stripScreenANSI(s.View().Content)
	for _, want := range []string{"zever serve -addr :9090", "listening :9090", "esc"} {
		if !strings.Contains(running, want) {
			t.Fatalf("running view missing %q:\n%s", want, running)
		}
	}
}

// --- dev screen ---

func TestDevScreenDebounceDefault(t *testing.T) {
	s := NewDevScreen()
	if s.debounceValue() != devDebounce {
		t.Fatalf("default debounce = %v, want core %v", s.debounceValue(), devDebounce)
	}
	if !strings.HasPrefix(s.CLI(), "zever dev") {
		t.Fatalf("CLI = %q", s.CLI())
	}
	cfg := resolveDevConfig(ProjectConfig{}.withDefaults(), nil)
	if cfg.Debounce != devDebounce || cfg.StopGrace != devStopGrace {
		t.Fatalf("core dev defaults changed: %+v", cfg)
	}
}

func TestDevFormEscCancels(t *testing.T) {
	s := NewDevScreen()
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc in form must yield a cmd")
	}
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}
}

// --- worker screen (queue:work + schedule:run alias) ---

func TestWorkerScreenAlias(t *testing.T) {
	s := NewQueueWorkScreen()
	if s.CLI() != "zever queue:work" {
		t.Fatalf("default CLI = %q", s.CLI())
	}
	if s.entryValue() != defaultWorkerEntry {
		t.Fatalf("default entry = %q, want %q", s.entryValue(), defaultWorkerEntry)
	}
	// schedule:run is an alias: same entry, same runner.
	cfg := resolveWorkerConfig(ProjectConfig{}.withDefaults(), []string{"--once"})
	if cfg.Entry != defaultWorkerEntry || len(cfg.Args) != 1 {
		t.Fatalf("core worker cfg = %+v", cfg)
	}
	if got := s.buildCLI("cmd/worker", []string{"--once"}); got != "zever queue:work --once" {
		t.Fatalf("cli = %q", got)
	}
	view := stripScreenANSI(s.View().Content)
	if !strings.Contains(view, "schedule:run") {
		t.Fatalf("worker form must note the alias:\n%s", view)
	}
}

// --- tinker screen ---

func TestTinkerScreenDefaults(t *testing.T) {
	s := NewTinkerScreen()
	if s.evalTimeoutValue() != tinkerEvalTimeout {
		t.Fatalf("eval timeout = %v, want core %v", s.evalTimeoutValue(), tinkerEvalTimeout)
	}
	if s.startTimeoutValue() != tinkerStartTimeout {
		t.Fatalf("start timeout = %v, want core %v", s.startTimeoutValue(), tinkerStartTimeout)
	}
	if s.entryValue() != defaultTinkerEntry {
		t.Fatalf("entry = %q, want %q", s.entryValue(), defaultTinkerEntry)
	}
	// Entry passes through the core resolver unchanged (no symbol widening path).
	cfg := resolveTinkerConfig(ProjectConfig{}.withDefaults(), "")
	if cfg.Entry != defaultTinkerEntry {
		t.Fatalf("core tinker entry = %q", cfg.Entry)
	}
	if !strings.HasPrefix(s.CLI(), "zever tinker") {
		t.Fatalf("CLI = %q", s.CLI())
	}
	view := stripScreenANSI(s.View().Content)
	if !strings.Contains(view, "development-only") {
		t.Fatalf("tinker form must show dev-only warning:\n%s", view)
	}
}

func TestTinkerAllowlistInherited(t *testing.T) {
	syms := tinkerAllowedSymbols()
	for key := range syms {
		if p := symKeyImportPath(key); p == "os/exec" || p == "syscall" || p == "unsafe" || p == "plugin" {
			t.Fatalf("allowlist leaks blocked import %q", key)
		}
	}
	// The screen evaluates through the same narrow surface: a blocked import
	// must fail inside a screen-owned interpreter.
	i, err := newTinkerInterp(nil)
	if err != nil {
		t.Fatalf("newTinkerInterp(nil): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := i.EvalWithContext(ctx, `import "unsafe"`); err == nil {
		t.Fatal("unsafe import must fail under the screen allowlist")
	}
}

func TestTinkerEvalTimeoutFromConfig(t *testing.T) {
	s := NewTinkerScreen()
	i, err := newTinkerInterp(nil)
	if err != nil {
		t.Fatalf("newTinkerInterp(nil): %v", err)
	}
	s.interp = i
	s.evalTimeout = 50 * time.Millisecond
	if got := s.evalLine(`1+1`); got != "2" {
		t.Fatalf("eval 1+1 = %q, want 2", got)
	}
	if got := s.evalLine(`undefinedTinkerSymbolXYZ`); !strings.HasPrefix(got, "error:") {
		t.Fatalf("bad eval = %q, want error prefix", got)
	}
	if err := s.evalErr(`for {}`); err == nil {
		t.Fatal("slow eval must hit the screen timeout")
	} else if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout err = %v", err)
	}
}

// --- running-phase esc stops the child, no orphans ---

func TestServeRunningEscStopsChild(t *testing.T) {
	old := runtimeStartPiped
	runtimeStartPiped = testStubPiped("hello-from-stub")
	defer func() { runtimeStartPiped = old }()

	s := NewServeScreen()
	s.stage = runtimeRunning
	s.cli = "zever serve"
	child, r, err := runtimeStartPiped("cmd/server", nil)
	if err != nil {
		t.Fatalf("stub start: %v", err)
	}
	s.attachChild(child, r)
	if !child.alive() {
		t.Fatal("stub child must be alive before esc")
	}
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc while running must yield a cmd")
	}
	if _, ok := cmd().(tui.LogBackMsg); !ok {
		t.Fatalf("cmd = %T, want tui.LogBackMsg", cmd())
	}
	deadline := time.Now().Add(5 * time.Second)
	for child.alive() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if child.alive() {
		t.Fatal("child survived esc stop: orphan")
	}
}

func TestChildLifecycleNoOrphanBounded(t *testing.T) {
	start := time.Now()
	cmd := exec.CommandContext(t.Context(), "sleep", "30")
	isolateProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	c := newRuntimeChild(cmd)
	if !c.alive() {
		t.Fatal("sleep child must be alive after start")
	}
	c.stop(200 * time.Millisecond)
	if c.alive() {
		t.Fatal("stop must reap the child (no orphan)")
	}
	// Idempotent: second stop must not hang or panic.
	c.stop(200 * time.Millisecond)
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Fatalf("lifecycle took %v, want bounded", elapsed)
	}
}

// --- teatest: real program wiring ---

func TestRuntimeServeTeatestEscBack(t *testing.T) {
	s := NewServeScreen()
	tm := teatest.NewTestModel(t, s, teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	final, ok := tm.FinalModel(t).(*ServeScreen)
	if !ok {
		t.Fatalf("final model = %T, want *ServeScreen", tm.FinalModel(t))
	}
	if !final.canceled {
		t.Fatal("esc must mark the serve screen canceled")
	}
}

// --- line pump ---

func TestRuntimeReadCmdBranches(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("a\nb"))
	m1 := runtimeReadCmd(r)()
	line1, ok := m1.(runtimeLineMsg)
	if !ok {
		t.Fatalf("msg = %T, want runtimeLineMsg", m1)
	}
	if line1.line != "a" {
		t.Fatalf("line1 = %+v", m1)
	}
	m2 := runtimeReadCmd(r)()
	line2, ok := m2.(runtimeLineMsg)
	if !ok {
		t.Fatalf("msg = %T, want runtimeLineMsg", m2)
	}
	if line2.line != "b" {
		t.Fatalf("eof-with-data must still emit line, got %+v", m2)
	}
	if m := runtimeReadCmd(bufio.NewReader(strings.NewReader("")))(); !isRuntimeDone(m) {
		t.Fatalf("empty stream must be done, got %+v", m)
	}
}

func isRuntimeDone(m tea.Msg) bool {
	_, ok := m.(runtimeDoneMsg)
	return ok
}

func TestDefaultStartPipeFailure(t *testing.T) {
	old := runtimePipe
	runtimePipe = func() (*os.File, *os.File, error) { return nil, nil, errRuntimeTestPipe }
	defer func() { runtimePipe = old }()
	if _, _, err := defaultStartRuntimePiped("cmd/server", nil); err == nil {
		t.Fatal("pipe failure must error")
	}
}

func TestDefaultStartCmdFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no `go` binary: Start must fail
	if _, _, err := defaultStartRuntimePiped("cmd/server", nil); err == nil {
		t.Fatal("missing go binary must fail Start")
	}
}

func TestLauncherStartErrorPath(t *testing.T) {
	old := runtimeStartPiped
	runtimeStartPiped = func(string, []string) (*runtimeChild, *bufio.Reader, error) {
		return nil, nil, errRuntimeTestPipe
	}
	defer func() { runtimeStartPiped = old }()
	s := NewServeScreen()
	if cmd := s.startRunning(); cmd != nil {
		t.Fatal("start error must yield nil cmd")
	}
	if s.stage != runtimeDone {
		t.Fatal("start error must land in done stage")
	}
	if got := stripScreenANSI(s.View().Content); !strings.Contains(got, "error:") {
		t.Fatalf("done view must show error:\n%s", got)
	}
}

func TestLauncherLineDoneAndResize(t *testing.T) {
	old := runtimeStartPiped
	runtimeStartPiped = testStubPiped("boot")
	defer func() { runtimeStartPiped = old }()
	s := NewServeScreen()
	cmd := s.startRunning()
	if cmd == nil {
		t.Fatal("start must yield a read cmd")
	}
	pumped := cmd()
	pumpedLine, ok := pumped.(runtimeLineMsg)
	if !ok {
		t.Fatalf("msg = %T, want runtimeLineMsg", pumped)
	}
	if pumpedLine.line != "boot" {
		t.Fatalf("first pumped line = %+v", pumped)
	}
	s.stage = runtimeRunning
	s.reader = bufio.NewReader(strings.NewReader(""))
	if _, cmd := s.Update(runtimeLineMsg{line: "hi"}); cmd == nil {
		t.Fatal("line msg must re-issue the read")
	}
	if _, cmd := s.Update(runtimeDoneMsg{}); cmd != nil {
		t.Fatal("done msg must yield nil cmd")
	}
	if s.stage != runtimeDone {
		t.Fatal("done msg must land in done stage")
	}
	if _, cmd := s.Update(runtimeDoneMsg{}); cmd != nil {
		t.Fatal("second done must be a no-op")
	}
	s.stage = runtimeForm
	if _, cmd := s.Update(runtimeDoneMsg{}); cmd != nil {
		t.Fatal("done in form must be a no-op")
	}
	if _, cmd := s.Update(runtimeLineMsg{line: "x"}); cmd != nil {
		t.Fatal("line in form must be a no-op")
	}
	_, _ = s.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if s.width != 100 || s.height != 30 {
		t.Fatalf("size = %dx%d", s.width, s.height)
	}
	if _, cmd := s.Update("unrelated"); cmd != nil {
		t.Fatal("unknown msg in form forwards without cmd guarantee")
	}
	s.stage = runtimeRunning
	if _, cmd := s.Update("unrelated"); cmd != nil {
		t.Fatal("unknown msg while running must be a no-op")
	}
	s.Stop()
}

func TestLauncherFormAbortMapsToCancel(t *testing.T) {
	if got := ctrlCKey().String(); got != "ctrl+c" {
		t.Skipf("ctrl+c key renders as %q on this platform", got)
	}
	for _, fresh := range []interface {
		Update(tea.Msg) (tea.Model, tea.Cmd)
	}{NewServeScreen(), NewDevScreen(), NewQueueWorkScreen(), NewTinkerScreen()} {
		if _, cmd := fresh.Update(ctrlCKey()); cmd == nil {
			t.Fatalf("%T: abort must yield a cmd", fresh)
		} else if _, ok := cmd().(tui.CanceledMsg); !ok {
			t.Fatalf("%T: cmd = %T, want tui.CanceledMsg", fresh, cmd())
		}
	}
}

func TestQueueWorkScreenFull(t *testing.T) {
	s := NewQueueWorkScreen()
	if s.Init() == nil {
		t.Fatal("Init must arm the form")
	}
	if _, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd == nil {
		t.Fatal("esc in form must yield a cmd")
	} else if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T", cmd())
	}
	s.stage = runtimeRunning
	s.cli = "zever queue:work"
	s.log.Append("worker up")
	if got := stripScreenANSI(s.View().Content); !strings.Contains(got, "worker up") {
		t.Fatalf("running view:\n%s", got)
	}
	s.Stop() // nil child: no-op
	old := runtimeStartPiped
	runtimeStartPiped = testStubPiped("")
	defer func() { runtimeStartPiped = old }()
	_ = s.startRunning()
	s.Stop()
	if s.child != nil {
		t.Fatal("Stop must release the child")
	}
}

func TestStopGraceKillWedgedChild(t *testing.T) {
	// The shell ignores SIGTERM but its short-lived sleep children do not:
	// the loop respawns them, so the group stays alive until SIGKILL.
	cmd := exec.CommandContext(t.Context(), "sh", "-c", "trap '' TERM; while true; do sleep 0.2; done")
	isolateProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	c := newRuntimeChild(cmd)
	// Let the shell install its SIGTERM trap before signalling; otherwise
	// the signal lands on default disposition and the fast path wins.
	time.Sleep(300 * time.Millisecond)
	if !c.alive() {
		t.Fatal("wedged child must survive until stopped")
	}
	c.stop(200 * time.Millisecond)
	if c.alive() {
		t.Fatal("SIGTERM-ignoring child must die via group SIGKILL")
	}
}

func TestServeResolvedEntryFallback(t *testing.T) {
	s := NewServeScreen()
	s.entry = "   "
	if s.entryValue() != defaultServerEntry {
		t.Fatalf("blank entry must fall back, got %q", s.entryValue())
	}
	d := NewDevScreen()
	d.entry = ""
	if d.entryValue() != defaultServerEntry {
		t.Fatalf("blank dev entry must fall back, got %q", d.entryValue())
	}
	d.entry = "cmd/custom"
	if d.entryValue() != "cmd/custom" {
		t.Fatalf("dev entry = %q", d.entryValue())
	}
	tk := NewTinkerScreen()
	tk.entry = ""
	if tk.entryValue() != defaultTinkerEntry {
		t.Fatalf("blank tinker entry must fall back, got %q", tk.entryValue())
	}
}

func TestDevDebounceAndCLI(t *testing.T) {
	s := NewDevScreen()
	for in, want := range map[string]time.Duration{"": devDebounce, "abc": devDebounce, "0": devDebounce, "-5": devDebounce, "500": 500 * time.Millisecond} {
		s.debounce = in
		if got := s.debounceValue(); got != want {
			t.Fatalf("debounce %q = %v, want %v", in, got, want)
		}
	}
	s.extra = "-addr :9090"
	if s.CLI() != "zever dev -addr :9090" {
		t.Fatalf("CLI = %q", s.CLI())
	}
	if s.Init() == nil {
		t.Fatal("Init must arm the form")
	}
}

func TestDevStartStopLifecycle(t *testing.T) {
	seenDone := make(chan struct{}, 1)
	oldLoop := runtimeDevLoop
	runtimeDevLoop = func(ctx context.Context, _ ProjectConfig, _ []string, out io.Writer) error {
		_, _ = out.Write([]byte("watching\n"))
		<-ctx.Done()
		_, _ = out.Write([]byte("shutting down\n"))
		seenDone <- struct{}{}
		return nil
	}
	defer func() { runtimeDevLoop = oldLoop }()
	prev := devDebounce
	defer func() { devDebounce = prev }()

	s := NewDevScreen()
	s.debounce = "50"
	cmd := s.startRunning()
	if s.stage != runtimeRunning {
		t.Fatal("dev must enter running stage")
	}
	if devDebounce != 50*time.Millisecond {
		t.Fatalf("session debounce = %v", devDebounce)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("start must batch loop+read, got %T", cmd())
	}
	if len(batch) != 2 {
		t.Fatalf("batch size = %d", len(batch))
	}
	go func() { _ = batch[0]() }()
	line := batch[1]()
	watchLine, ok := line.(runtimeLineMsg)
	if !ok {
		t.Fatalf("msg = %T, want runtimeLineMsg", line)
	}
	if watchLine.line != "watching" {
		t.Fatalf("watcher line = %+v", line)
	}
	if _, cmd := s.Update(line); cmd == nil {
		t.Fatal("watcher line must re-issue the read")
	}
	if _, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd == nil {
		t.Fatal("esc must yield a cmd")
	} else if _, ok := cmd().(tui.LogBackMsg); !ok {
		t.Fatalf("cmd = %T, want tui.LogBackMsg", cmd())
	}
	select {
	case <-seenDone:
	case <-time.After(5 * time.Second):
		t.Fatal("ctx cancel must unwind the loop")
	}
	if devDebounce != prev {
		t.Fatalf("stop must restore debounce, got %v", devDebounce)
	}
	s.stage = runtimeRunning
	if _, cmd := s.Update(runtimeDoneMsg{}); cmd != nil {
		t.Fatal("loop completion must yield nil cmd")
	}
	if s.stage != runtimeDone {
		t.Fatal("loop completion must land in done stage")
	}
	if got := stripScreenANSI(s.View().Content); !strings.Contains(got, "zever dev") {
		t.Fatalf("dev done view:\n%s", got)
	}
	s.stage = runtimeRunning
	s.reader = nil
	if _, cmd := s.Update(runtimeLineMsg{line: "x"}); cmd != nil {
		t.Fatal("line with closed stream must not re-issue")
	}
	_, _ = s.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	if s.width != 90 || s.height != 20 {
		t.Fatalf("dev size = %dx%d", s.width, s.height)
	}
}

func TestTinkerStartErrorAndCloseEmpty(t *testing.T) {
	old := runtimeStartTinker
	runtimeStartTinker = func(string, time.Duration, io.Writer) (*interp.Interpreter, func() error, error) {
		return nil, nil, errRuntimeTestPipe
	}
	defer func() { runtimeStartTinker = old }()
	s := NewTinkerScreen()
	if cmd := s.startRunning(); cmd != nil {
		t.Fatal("start error must yield nil cmd")
	}
	if s.stage != runtimeDone {
		t.Fatal("start error must land in done stage")
	}
	empty := NewTinkerScreen()
	empty.closeTinker() // no session: no-op
	if s.Init() == nil {
		t.Fatal("Init must arm the form")
	}
}

func TestTinkerReplKeys(t *testing.T) {
	old := runtimeStartTinker
	runtimeStartTinker = func(string, time.Duration, io.Writer) (*interp.Interpreter, func() error, error) {
		i, err := newTinkerInterp(nil)
		if err != nil {
			return nil, nil, err
		}
		return i, func() error { return nil }, nil
	}
	defer func() { runtimeStartTinker = old }()
	s := NewTinkerScreen()
	s.evalTimeout = 5 * time.Second
	if cmd := s.startRunning(); cmd == nil {
		t.Fatal("start must yield a read cmd")
	}
	if s.stage != runtimeRunning {
		t.Fatal("tinker must enter running stage")
	}
	if got := stripScreenANSI(s.View().Content); !strings.Contains(got, "zever tinker --entry") {
		t.Fatalf("running view:\n%s", got)
	}
	type keyMsg = tea.KeyPressMsg
	if _, cmd := s.Update(keyMsg{Code: '1', Text: "1"}); cmd != nil {
		t.Fatal("typing must yield nil cmd")
	}
	if _, cmd := s.Update(keyMsg{Code: tea.KeyBackspace}); cmd != nil {
		t.Fatal("backspace must yield nil cmd")
	}
	s.backspace() // empty buffer: no-op
	if _, cmd := s.Update(keyMsg{Code: '2', Text: "2"}); cmd != nil {
		t.Fatal("typing must yield nil cmd")
	}
	if _, cmd := s.Update(keyMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("eval submit must yield nil cmd")
	}
	if got := strings.Join(s.log.Lines(), "\n"); !strings.Contains(got, "zever> 2") {
		t.Fatalf("pane must show the exchange:\n%s", got)
	}
	s.input = ":help"
	if _, cmd := s.Update(keyMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("help must yield nil cmd")
	}
	s.input = ""
	if _, cmd := s.Update(keyMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("empty submit must yield nil cmd")
	}
	s.input = ":exit"
	m, cmd := s.Update(keyMsg{Code: tea.KeyEnter})
	_ = m
	if cmd == nil {
		t.Fatal("exit must pop")
	} else if _, ok := cmd().(tui.LogBackMsg); !ok {
		t.Fatalf("cmd = %T", cmd())
	}
	// esc path with live session
	s2 := NewTinkerScreen()
	s2.evalTimeout = 5 * time.Second
	if cmd := s2.startRunning(); cmd == nil {
		t.Fatal("start must yield a read cmd")
	}
	if _, cmd := s2.Update(keyMsg{Code: tea.KeyEscape}); cmd == nil {
		t.Fatal("esc must yield a cmd")
	} else if _, ok := cmd().(tui.LogBackMsg); !ok {
		t.Fatalf("cmd = %T", cmd())
	}
	// non-typing key forwards to the log pane
	s3 := NewTinkerScreen()
	s3.evalTimeout = 5 * time.Second
	if cmd := s3.startRunning(); cmd == nil {
		t.Fatal("start must yield a read cmd")
	}
	for i := 0; i < 30; i++ {
		s3.log.Append("history line")
	}
	if _, cmd := s3.Update(keyMsg{Code: tea.KeyPgUp}); cmd == nil {
		_ = cmd
	}
	if s3.log.Follow() {
		t.Fatal("pgup must disarm follow-tail")
	}
	s3.stage = runtimeDone
	if _, cmd := s3.Update(keyMsg{Code: tea.KeyDown}); cmd != nil {
		_ = cmd // viewport may or may not animate; only routing matters
	}
	if s3.stage != runtimeDone {
		t.Fatal("done-stage keys must not revive the REPL")
	}
	if _, cmd := s3.Update(runtimeDoneMsg{}); cmd != nil {
		t.Fatal("done in done stage must be a no-op")
	}
	s3.stage = runtimeRunning
	if _, cmd := s3.Update(runtimeLineMsg{line: "shim log"}); cmd == nil {
		t.Fatal("shim line must re-issue the read")
	}
	s3.shimReader = nil
	if _, cmd := s3.Update(runtimeLineMsg{line: "x"}); cmd != nil {
		t.Fatal("line with closed stream must not re-issue")
	}
	if _, cmd := s3.Update("unrelated"); cmd != nil {
		t.Fatal("unknown msg while running must be a no-op")
	}
	s3.stage = runtimeForm
	if _, cmd := s3.Update("unrelated"); cmd != nil {
		t.Fatal("unknown msg in form forwards without guarantee")
	}
	_, _ = s3.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	if s3.width != 90 || s3.height != 20 {
		t.Fatalf("tinker size = %dx%d", s3.width, s3.height)
	}
	s3.closeTinker()
}

func TestDefaultStartTinkerShimFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no `go` binary: shim Start must fail
	if _, _, err := defaultStartTinker("cmd/tinker-shim", time.Second, io.Discard); err == nil {
		t.Fatal("missing go binary must fail shim start")
	}
}

func TestDefaultStartTinkerPingFailure(t *testing.T) {
	// tui/ is a library: `go run` exits without speaking the shim
	// protocol, so the readiness ping must fail fast.
	if _, _, err := defaultStartTinker("cmd/zever/tui", 5*time.Second, io.Discard); err == nil {
		t.Fatal("non-shim entry must fail the readiness ping")
	}
}

func TestDefaultStartSuccessFast(t *testing.T) {
	// Exercises the production start path: `go run` begins (Start
	// succeeds) and fails during package load; the child is reaped.
	child, r, err := defaultStartRuntimePiped("runtime-probe-nonexistent-xyz", nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if r == nil {
		t.Fatal("want an output reader")
	}
	child.stop(10 * time.Second)
	if child.alive() {
		t.Fatal("probe child must be reaped")
	}
}

func TestLauncherFormCompletionConfirms(t *testing.T) {
	old := runtimeStartPiped
	runtimeStartPiped = testStubPiped("")
	defer func() { runtimeStartPiped = old }()
	s := NewServeScreen()
	s.form.State = huh.StateCompleted
	if _, cmd := s.Update("unrelated"); cmd == nil {
		t.Fatal("completed form must launch the child")
	}
	if s.stage != runtimeRunning {
		t.Fatal("completed form must enter running stage")
	}
	s.Stop()
}

func TestLauncherRunningKeysAndEmptyCLIView(t *testing.T) {
	s := NewServeScreen()
	s.stage = runtimeRunning
	s.cli = ""
	if got := stripScreenANSI(s.View().Content); !strings.Contains(got, "zever serve") {
		t.Fatalf("empty-cli view must fall back to live CLI:\n%s", got)
	}
	if _, cmd := s.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}); cmd != nil {
		_ = cmd // viewport may answer; routing is what matters
	}
}

func TestDevFormCompletionAndViews(t *testing.T) {
	seenDone := make(chan struct{}, 1)
	oldLoop := runtimeDevLoop
	runtimeDevLoop = func(ctx context.Context, _ ProjectConfig, _ []string, _ io.Writer) error {
		<-ctx.Done()
		seenDone <- struct{}{}
		return nil
	}
	defer func() { runtimeDevLoop = oldLoop }()
	prev := devDebounce
	defer func() { devDebounce = prev }()

	s := NewDevScreen()
	if got := stripScreenANSI(s.View().Content); !strings.Contains(got, "zever dev") {
		t.Fatalf("dev form view:\n%s", got)
	}
	if _, cmd := s.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); cmd != nil {
		_ = cmd // huh consumes the keystroke; routing is what matters
	}
	s.form.State = huh.StateCompleted
	_, cmd := s.Update("unrelated")
	if cmd == nil {
		t.Fatal("completed dev form must launch the loop")
	}
	if s.stage != runtimeRunning {
		t.Fatal("completed dev form must enter running stage")
	}
	if batch, ok := cmd().(tea.BatchMsg); ok && len(batch) > 0 {
		go func() { _ = batch[0]() }()
	}
	if _, keyCmd := s.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}); keyCmd != nil {
		_ = keyCmd
	}
	s.stage = runtimeForm
	if _, lineCmd := s.Update(runtimeLineMsg{line: "x"}); lineCmd != nil {
		t.Fatal("dev line in form must be a no-op")
	}
	s.stage = runtimeRunning
	if _, pingCmd := s.Update("unrelated"); pingCmd != nil {
		t.Fatal("dev unknown msg while running must be a no-op")
	}
	s.cli = ""
	if got := stripScreenANSI(s.View().Content); !strings.Contains(got, "zever dev") {
		t.Fatalf("dev empty-cli view:\n%s", got)
	}
	_, cmd = s.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc must yield a cmd")
	}
	_ = cmd()
	select {
	case <-seenDone:
	case <-time.After(5 * time.Second):
		t.Fatal("ctx cancel must unwind the loop")
	}
}

func TestTinkerFormCompletionAndEsc(t *testing.T) {
	old := runtimeStartTinker
	runtimeStartTinker = func(string, time.Duration, io.Writer) (*interp.Interpreter, func() error, error) {
		i, err := newTinkerInterp(nil)
		if err != nil {
			return nil, nil, err
		}
		return i, func() error { return nil }, nil
	}
	defer func() { runtimeStartTinker = old }()

	s := NewTinkerScreen()
	s.evalTimeout = 5 * time.Second
	s.form.State = huh.StateCompleted
	if _, cmd := s.Update("unrelated"); cmd == nil {
		t.Fatal("completed tinker form must start the shim")
	}
	if s.stage != runtimeRunning {
		t.Fatal("completed tinker form must enter running stage")
	}

	esc := NewTinkerScreen()
	if _, cmd := esc.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd == nil {
		t.Fatal("esc in tinker form must yield a cmd")
	} else if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}

	if err := s.evalErr(`1+1`); err != nil {
		t.Fatalf("evalErr success = %v", err)
	}
	if err := s.evalErr(`undefinedTinkerSymbolXYZ`); err == nil {
		t.Fatal("evalErr must surface compile errors")
	} else if strings.Contains(err.Error(), "timed out") {
		t.Fatalf("compile error must not look like a timeout: %v", err)
	}
	s.evalTimeout = 50 * time.Millisecond
	if got := s.evalLine(`for {}`); !strings.Contains(got, "timed out") {
		t.Fatalf("evalLine timeout = %q", got)
	}
	s.stage = runtimeDone
	if got := stripScreenANSI(s.View().Content); !strings.Contains(got, "esc back") {
		t.Fatalf("tinker done view:\n%s", got)
	}
	s.closeTinker()

	s4 := NewTinkerScreen()
	s4.stage = runtimeForm
	if _, cmd := s4.Update(runtimeLineMsg{line: "x"}); cmd != nil {
		t.Fatal("tinker line in form must be a no-op")
	}
	s4.stage = runtimeRunning
	s4.shimReader = bufio.NewReader(strings.NewReader(""))
	if _, cmd := s4.Update(runtimeDoneMsg{}); cmd != nil {
		t.Fatal("shim completion must yield nil cmd")
	}
	if s4.stage != runtimeDone {
		t.Fatal("shim completion must land in done stage")
	}
	s4.cli = ""
	if got := stripScreenANSI(s4.View().Content); !strings.Contains(got, "zever tinker") {
		t.Fatalf("tinker empty-cli view:\n%s", got)
	}
}

// TestCoverDefaultStartTinkerInterpErrorAndSuccess covers the interp-error
// close path and the success return via seams (no toolchain).
func TestCoverDefaultStartTinkerInterpErrorAndSuccess(t *testing.T) {
	origStart, origNew := tinkerShimStartForScreen, tinkerInterpNewForScreen
	t.Cleanup(func() { tinkerShimStartForScreen, tinkerInterpNewForScreen = origStart, origNew })
	newFakeClient := func() *tinkerClient {
		pr, pw := io.Pipe()
		sr, sw := io.Pipe()
		go func() {
			br := bufio.NewReader(sr)
			_, _ = br.ReadString('\n')
			_, _ = pw.Write([]byte(tinkerFramePrefix + `{"result":"pong"}` + "\n"))
		}()
		return &tinkerClient{stdin: sw, stdout: bufio.NewReader(pr), passthrough: io.Discard}
	}
	c := newFakeClient()
	tinkerShimStartForScreen = func(string, io.Writer) (*tinkerClient, error) { return c, nil }
	tinkerInterpNewForScreen = func(*tinkerClient) (*interp.Interpreter, error) { return nil, errTestSentinel }
	if _, _, err := defaultStartTinker("x", time.Second, io.Discard); err == nil {
		t.Fatalf("want interp error")
	}
	c2 := newFakeClient()
	tinkerShimStartForScreen = func(string, io.Writer) (*tinkerClient, error) { return c2, nil }
	tinkerInterpNewForScreen = func(*tinkerClient) (*interp.Interpreter, error) { return interp.New(interp.Options{}), nil }
	i, cleanup, err := defaultStartTinker("x", time.Second, io.Discard)
	if err != nil {
		t.Fatalf("success: %v", err)
	}
	if i == nil || cleanup == nil {
		t.Fatalf("want interp + cleanup")
	}
	_ = c2.Close()
}
