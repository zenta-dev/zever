package main

// Tests for ExtractScreen: config/CLI/preview rendering, stage transitions,
// golden views, the plan-then-write run path (failure plus a real temp
// extraction), and the single teatest end-to-end program run for this group.

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

// TestExtractScreenTeatestEndToEnd is the group's single full-program run:
// boot the screen, type a module name, and walk the form with synthetic
// keys. It lands on preview (or exec if a confirm key overruns); either way
// the frozen CLI proves end-to-end wiring. Confirming here is write-free:
// with no schema workspace around, the extract plan fails before any write.
func TestExtractScreenTeatestEndToEnd(t *testing.T) {
	s := NewExtractScreen()

	tm := teatest.NewTestModel(t, s, teatest.WithInitialTermSize(80, 24))
	time.Sleep(300 * time.Millisecond)
	for _, r := range "billing" {
		tm.Send(scaffoldKey(string(r)))
		time.Sleep(50 * time.Millisecond)
	}
	for i := 0; i < 6; i++ {
		tm.Send(scaffoldKey("enter"))
		time.Sleep(200 * time.Millisecond)
	}
	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	final, ok := tm.FinalModel(t).(*ExtractScreen)
	if !ok {
		t.Fatalf("final model = %T, want *ExtractScreen", tm.FinalModel(t))
	}

	cli := final.flow.cli
	if cli == "" && final.flow.hasExec {
		cli = final.flow.exec.CLI()
	}
	if cli != "zever extract billing" {
		t.Fatalf("stage = %d CLI = %q, want preview/exec with CLI %q", final.flow.stage, cli, "zever extract billing")
	}
	if final.flow.stage != stagePreview && final.flow.stage != stageExec {
		t.Fatalf("stage = %d, want preview or exec", final.flow.stage)
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
