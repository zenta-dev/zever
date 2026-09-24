package main

// Tests for ExtractScreen: config/CLI/preview rendering, stage transitions,
// golden views, the plan-then-write run path (failure plus a real temp
// extraction), and the single teatest end-to-end program run for this group.

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// scaffoldExtractWithValues returns a screen with a valid module baked
// into a rebuilt form.
func scaffoldExtractWithValues() *ExtractScreen {
	m := NewExtractScreen()
	m.module = "shop"
	m.flow.form = m.buildForm()

	return m
}

// scaffoldExtractToPreview walks the form to preview with synthetic
// enters, pumping huh's focus handoffs like a real runtime would.
func scaffoldExtractToPreview(t *testing.T, m *ExtractScreen) {
	t.Helper()

	var asModel tea.Model = m
	for i := 0; i < 12 && m.flow.stage == stageForm; i++ {
		asModel = scaffoldTapEnter(t, asModel)
		var ok bool
		m, ok = asModel.(*ExtractScreen)
		if !ok {
			t.Fatalf("model = %T, want *ExtractScreen", asModel)
		}
	}

	if m.flow.stage != stagePreview {
		t.Fatalf("stage = %d, want preview", m.flow.stage)
	}
}

func TestExtractScreenConfigAndCLI(t *testing.T) {
	m := scaffoldExtractWithValues()

	cfg := m.buildConfig()
	if cfg.Module != "shop" || cfg.OutDir != "" || cfg.ModulePath != "" || cfg.Force {
		t.Fatalf("config = %+v", cfg)
	}
	if got := extractScreenCLI(cfg); got != "zever extract shop" {
		t.Fatalf("CLI = %q", got)
	}

	m.outDir = "./shop-service"
	m.modulePath = "example.com/shop-service"
	m.force = true

	cfg = m.buildConfig()
	if got := extractScreenCLI(cfg); got != "zever extract shop --out ./shop-service --module example.com/shop-service --force" {
		t.Fatalf("CLI = %q", got)
	}

	lines := extractScreenPreview(cfg)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"module: shop", "out: ./shop-service", "force: true"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("preview lacks %q:\n%s", want, joined)
		}
	}

	m2 := scaffoldExtractWithValues()
	m2.freezePreview()
	if m2.flow.cli != "zever extract shop" {
		t.Fatalf("frozen CLI = %q", m2.flow.cli)
	}
	if !strings.Contains(strings.Join(m2.flow.preview, "\n"), "shop-service (default)") {
		t.Fatalf("default out missing:\n%s", strings.Join(m2.flow.preview, "\n"))
	}
}

func TestExtractScreenEscAndConfirm(t *testing.T) {
	m := scaffoldExtractWithValues()

	if m.Init() == nil {
		t.Fatal("Init must arm the form")
	}

	_, cmd := scaffoldUpdate(t, m, scaffoldKey("esc"))
	if msg := scaffoldCmdMsg(t, cmd); msg != (tui.BackMsg{}) {
		t.Fatalf("form esc = %#v, want BackMsg", msg)
	}

	scaffoldExtractToPreview(t, m)

	_, cmd = scaffoldUpdate(t, m, scaffoldKey("esc"))
	if msg := scaffoldCmdMsg(t, cmd); msg != (tui.BackMsg{}) {
		t.Fatalf("preview esc = %#v, want BackMsg", msg)
	}

	if _, keyCmd := scaffoldUpdate(t, m, scaffoldKey("j")); keyCmd != nil {
		t.Fatal("preview j must be a no-op")
	}

	_, cmd = scaffoldUpdate(t, m, scaffoldKey("enter"))
	if cmd == nil || m.flow.stage != stageExec {
		t.Fatal("preview enter must start exec")
	}

	_, _ = scaffoldUpdate(t, m, tui.ExecDoneMsg{Output: "extracted 4 files"})
	scaffoldContains(t, m.View().Content, "equivalent CLI: zever extract shop")

	_, _ = scaffoldUpdate(t, m, tui.ExecErrMsg{Err: tui.ErrCanceled})
	scaffoldContains(t, m.View().Content, "canceled")
}

func TestExtractScreenExecErrorView(t *testing.T) {
	m := scaffoldExtractWithValues()
	scaffoldExtractToPreview(t, m)

	_, _ = scaffoldUpdate(t, m, scaffoldKey("enter"))
	_, _ = scaffoldUpdate(t, m, tui.ExecErrMsg{Err: errors.New("no module")})
	scaffoldContains(t, m.View().Content, "no module")
}

func TestExtractScreenGoldenViews(t *testing.T) {
	m := scaffoldExtractWithValues()
	_, _ = scaffoldUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	scaffoldContains(t, m.View().Content, "Module to extract")

	scaffoldExtractToPreview(t, m)
	preview := stripScreenANSI(m.View().Content)
	for _, want := range []string{"zever extract", "module: shop", "equivalent CLI: zever extract shop"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("preview lacks %q:\n%s", want, preview)
		}
	}
}

func TestRunExtractScreenNoProject(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	cfg := ExtractConfig{Module: "shop"}
	if _, err := runExtractScreen(cfg); err == nil {
		t.Fatal("missing go.mod must error")
	}
}

func TestRunExtractScreenUnknownModule(t *testing.T) {
	dir := scaffoldExtractProject(t)

	cfg := ExtractConfig{Module: "warehouse", OutDir: dir + "/warehouse-service"}
	if _, err := runExtractScreen(cfg); err == nil {
		t.Fatal("unknown module must error")
	} else if !strings.Contains(err.Error(), `no module "warehouse"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestRunExtractScreenWritesService(t *testing.T) {
	scaffoldExtractProject(t)

	cfg := ExtractConfig{Module: "billing"}
	summary, err := runExtractScreen(cfg)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(summary, `"billing"`) {
		t.Fatalf("summary = %q", summary)
	}
}

// extractProbe wraps an ExtractScreen for the teatest run below,
// snapshotting race-safe observations after every Update so Sends can be
// paced on real outcomes (typed value, focus handoffs, stage) instead of
// fixed sleeps: teatest Sends are consumed asynchronously and huh
// completes focus handoffs via commands, so burst Sends overrun the
// focused field (the run stalls in form with an empty CLI).
type extractProbe struct {
	mu        sync.Mutex
	inner     *ExtractScreen
	module    string
	stage     scaffoldStage
	cli       string
	focused   any
	formState huh.FormState
}

func (p *extractProbe) Init() tea.Cmd { return p.inner.Init() }

func (p *extractProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := p.inner.Update(msg)
	es, _ := m.(*ExtractScreen)
	if es == nil {
		return p, cmd
	}
	p.mu.Lock()
	p.inner = es
	p.module = es.module
	p.stage = es.flow.stage
	p.cli = es.flow.cli
	if es.flow.form != nil {
		p.focused = es.flow.form.GetFocusedField()
		p.formState = es.flow.form.State
	}
	p.mu.Unlock()
	return p, cmd
}

func (p *extractProbe) View() tea.View {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inner.View()
}

// snapshot returns the latest observed wizard state.
func (p *extractProbe) snapshot() (module string, stage scaffoldStage, focused any, formState huh.FormState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.module, p.stage, p.focused, p.formState
}

// finalState returns the terminal stage and CLI for assertions,
// falling back to the exec model's CLI exactly like the unwrapped check.
func (p *extractProbe) finalState() (scaffoldStage, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cli := p.cli
	if cli == "" && p.inner.flow.hasExec {
		cli = p.inner.flow.exec.CLI()
	}
	return p.stage, cli
}

// TestExtractScreenTeatestEndToEnd is the group's single full-program run:
// boot the screen, type a module name, and walk the form with synthetic
// keys. It lands on preview (or exec if a confirm key overruns); either way
// the frozen CLI proves end-to-end wiring. Confirming here is write-free:
// with no schema workspace around, the extract plan fails before any write.
func TestExtractScreenTeatestEndToEnd(t *testing.T) {
	probe := &extractProbe{inner: NewExtractScreen()}

	tm := teatest.NewTestModel(t, probe, teatest.WithInitialTermSize(80, 24))
	// Wait for huh's Init commands to settle (focus established) before
	// typing: Init follow-ups are async, so early keys can land ahead of
	// them and leave the form in a state where Enter no-ops.
	pollFor(t, 15*time.Second, func() bool {
		_, _, focused, _ := probe.snapshot()
		return focused != nil
	})
	for _, r := range "billing" {
		tm.Send(scaffoldKey(string(r)))
	}
	// Wait for the typed value to land in the bound field.
	pollFor(t, 15*time.Second, func() bool {
		module, _, _, _ := probe.snapshot()
		return module == "billing"
	})
	// Walk the form: each enter's async handoff (focus move, group
	// submit, or the trailing key that flips a completed form to
	// preview) must land before the next enter is sent, otherwise keys
	// hit the wrong field or the completion is never observed.
	for i := 0; i < 12; i++ {
		_, stage, _, _ := probe.snapshot()
		if stage != stageForm {
			break
		}
		_, _, before, beforeState := probe.snapshot()
		tm.Send(scaffoldKey("enter"))
		pollFor(t, 15*time.Second, func() bool {
			_, stage, focused, formState := probe.snapshot()
			return stage != stageForm || focused != before || formState != beforeState
		})
	}
	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	final, ok := tm.FinalModel(t).(*extractProbe)
	if !ok {
		t.Fatalf("final model = %T, want *extractProbe", tm.FinalModel(t))
	}

	stage, cli := final.finalState()
	if cli != "zever extract billing" {
		t.Fatalf("stage = %d CLI = %q, want preview/exec with CLI %q", stage, cli, "zever extract billing")
	}
	if stage != stagePreview && stage != stageExec {
		t.Fatalf("stage = %d, want preview or exec", stage)
	}
}

func TestRunExtractScreenBrokenProject(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)
	scaffoldBrokenConfig(t)

	// loadProjectConfig fails before go.mod is even read.
	if _, err := runExtractScreen(ExtractConfig{Module: "shop"}); err == nil {
		t.Fatal("broken config must error")
	}
}

func TestRunExtractScreenBrokenSchema(t *testing.T) {
	dir := scaffoldExtractProject(t)

	// Uncompilable schema fails the compile step before planning.
	bad := "this is not zen {{{\n"
	if err := os.WriteFile(dir+"/schema/billing/billing.zen", []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runExtractScreen(ExtractConfig{Module: "billing"}); err == nil {
		t.Fatal("broken schema must error")
	}
}

func TestRunExtractScreenRefusesCollision(t *testing.T) {
	scaffoldExtractProject(t)

	cfg := ExtractConfig{Module: "billing"}
	if _, err := runExtractScreen(cfg); err != nil {
		t.Fatalf("extract: %v", err)
	}
	// Without force the written tree collides on the second run.
	if _, err := runExtractScreen(cfg); err == nil {
		t.Fatal("second run without force must error")
	}
}
