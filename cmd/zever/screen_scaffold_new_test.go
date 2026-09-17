package main

// Tests for NewScreen (`zever new` wizard): config resolution, CLI and
// preview rendering, stage transitions over synthetic keys, golden views at
// fixed 80x24, and the writeNewProject run path in temp dirs.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// scaffoldNewWithValues returns a screen with valid inputs baked into a
// rebuilt form, so views render them and validation passes.
func scaffoldNewWithValues() *NewScreen {
	m := NewScaffoldScreen()
	m.name = "myapp"
	m.flow.form = m.buildForm()

	return m
}

// scaffoldNewToPreview walks the form to preview with synthetic enters,
// pumping huh's focus handoffs like a real runtime would.
func scaffoldNewToPreview(t *testing.T, m *NewScreen) {
	t.Helper()

	var asModel tea.Model = m
	for i := 0; i < 20 && m.flow.stage == stageForm; i++ {
		asModel = scaffoldTapEnter(t, asModel)
		var ok bool
		m, ok = asModel.(*NewScreen)
		if !ok {
			t.Fatalf("model = %T, want *NewScreen", asModel)
		}
	}

	if m.flow.stage != stagePreview {
		t.Fatalf("stage = %d, want preview", m.flow.stage)
	}
}

func TestNewScreenDefaults(t *testing.T) {
	m := NewScaffoldScreen()

	if m.db != "sqlite" || m.cache != "memory" || m.queue != "memory" {
		t.Fatalf("adapter defaults = %s/%s/%s", m.db, m.cache, m.queue)
	}
	if len(m.batteries) != 0 || m.force {
		t.Fatalf("batteries/force defaults = %v/%v", m.batteries, m.force)
	}
	if len(m.backends) != len(defaultNewBackends) {
		t.Fatalf("backends = %v, want preselected defaults", m.backends)
	}
	if m.flow.stage != stageForm {
		t.Fatalf("initial stage = %d, want form", m.flow.stage)
	}
	if m.Init() == nil {
		t.Fatal("Init must arm the form")
	}
}

func TestNewScreenBuildConfig(t *testing.T) {
	m := scaffoldNewWithValues()
	m.module = ""
	m.outDir = ""

	cfg := m.buildConfig()
	if cfg.Name != "myapp" || cfg.OutDir != "./myapp" || cfg.ModulePath != "myapp" {
		t.Fatalf("defaults = %+v", cfg)
	}
	if cfg.DBAdapter != "" || cfg.CacheAdapter != "" || cfg.QueueAdapter != "" {
		t.Fatalf("default adapters must stay empty overrides: %+v", cfg)
	}

	m.db = "postgres"
	m.cache = "redis"
	m.queue = "redis"
	m.batteries = []string{"search"}
	m.force = true

	cfg = m.buildConfig()
	if cfg.DBAdapter != "postgres" || cfg.CacheAdapter != "redis" || cfg.QueueAdapter != "redis" {
		t.Fatalf("overrides = %+v", cfg)
	}
	found := map[string]bool{}
	for _, b := range cfg.Batteries {
		found[b] = true
	}
	if !found["search"] || !found["cache"] {
		t.Fatalf("batteries = %v, want search plus implied cache", cfg.Batteries)
	}
}

func TestNewScreenCLI(t *testing.T) {
	m := scaffoldNewWithValues()

	if got := newScreenCLI(m.buildConfig()); got != "zever new myapp" {
		t.Fatalf("CLI = %q", got)
	}

	m.module = "example.com/myapp"
	m.outDir = "./apps/myapp"
	m.force = true
	m.db = "postgres"

	if got := newScreenCLI(m.buildConfig()); got != "zever new myapp --dir ./apps/myapp --module example.com/myapp --force" {
		t.Fatalf("CLI = %q", got)
	}
}

func TestNewScreenPreview(t *testing.T) {
	m := scaffoldNewWithValues()
	m.freezePreview()

	joined := strings.Join(m.flow.preview, "\n")
	for _, want := range []string{"app: myapp", "dir: ./myapp", "sqlite", "memory"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("preview lacks %q:\n%s", want, joined)
		}
	}
	if m.flow.cli != "zever new myapp" {
		t.Fatalf("CLI = %q", m.flow.cli)
	}
}

func TestNewScreenEscBacksOut(t *testing.T) {
	m := scaffoldNewWithValues()

	_, cmd := scaffoldUpdate(t, m, scaffoldKey("esc"))
	if msg := scaffoldCmdMsg(t, cmd); msg != (tui.BackMsg{}) {
		t.Fatalf("esc = %#v, want BackMsg", msg)
	}
	if m.flow.stage != stageForm {
		t.Fatalf("esc must not advance stage: %d", m.flow.stage)
	}
}

func TestNewScreenInvalidNameBlocksCompletion(t *testing.T) {
	// Inline validation rejects the value before any core runs.
	if err := validateAppNameField("not valid"); err == nil {
		t.Fatal("invalid name must fail validation")
	}

	// An empty required name never completes, however often enter is hit.
	m := NewScaffoldScreen()
	var asModel tea.Model = m
	for i := 0; i < 4; i++ {
		asModel = scaffoldTapEnter(t, asModel)
	}
	newScreen, ok := asModel.(*NewScreen)
	if !ok {
		t.Fatalf("model = %T, want *NewScreen", asModel)
	}
	if newScreen.flow.stage != stageForm {
		t.Fatal("invalid form must stay open")
	}
}

func TestNewScreenFormToPreviewToExec(t *testing.T) {
	m := scaffoldNewWithValues()
	scaffoldNewToPreview(t, m)

	// Preview esc backs out without writing.
	_, cmd := scaffoldUpdate(t, m, scaffoldKey("esc"))
	if msg := scaffoldCmdMsg(t, cmd); msg != (tui.BackMsg{}) {
		t.Fatalf("preview esc = %#v, want BackMsg", msg)
	}

	// Unrelated keys are ignored on preview.
	if _, keyCmd := scaffoldUpdate(t, m, scaffoldKey("j")); keyCmd != nil {
		t.Fatal("preview j must be a no-op")
	}

	// Confirm starts the exec (the Start cmd is returned, not run here).
	if _, enterCmd := scaffoldUpdate(t, m, scaffoldKey("enter")); enterCmd == nil {
		t.Fatal("preview enter must start exec")
	}
	if m.flow.stage != stageExec || !m.flow.hasExec {
		t.Fatal("confirm must enter exec stage")
	}

	// Feed completion and error outcomes without touching disk.
	if _, doneCmd := scaffoldUpdate(t, m, tui.ExecDoneMsg{Output: "scaffolded 3 files"}); doneCmd != nil {
		t.Fatal("done must yield nil cmd")
	}
	scaffoldContains(t, m.View().Content, "equivalent CLI: zever new myapp")
	scaffoldContains(t, m.View().Content, "scaffolded 3 files")

	// esc after done backs out.
	_, cmd = scaffoldUpdate(t, m, scaffoldKey("esc"))
	if msg := scaffoldCmdMsg(t, cmd); msg != (tui.BackMsg{}) {
		t.Fatalf("exec-done esc = %#v, want BackMsg", msg)
	}
}

func TestNewScreenExecFailedAndCanceledViews(t *testing.T) {
	m := scaffoldNewWithValues()
	scaffoldNewToPreview(t, m)

	_, _ = scaffoldUpdate(t, m, scaffoldKey("enter"))

	boom := errors.New("mkdir denied")
	_, _ = scaffoldUpdate(t, m, tui.ExecErrMsg{Err: boom})
	scaffoldContains(t, m.View().Content, "mkdir denied")
	scaffoldContains(t, m.View().Content, "equivalent CLI:")

	// esc while running cancels through the exec model.
	m2 := scaffoldNewWithValues()
	scaffoldNewToPreview(t, m2)
	_, cmd := scaffoldUpdate(t, m2, scaffoldKey("enter"))
	_ = cmd
	_, cancelCmd := scaffoldUpdate(t, m2, scaffoldKey("esc"))
	if msg := scaffoldCmdMsg(t, cancelCmd); msg != (tui.CanceledMsg{}) {
		t.Fatalf("exec esc = %#v, want CanceledMsg", msg)
	}

	// Window size and stray messages are safe everywhere.
	if _, cmd := scaffoldUpdate(t, m2, tea.WindowSizeMsg{Width: 80, Height: 24}); cmd != nil {
		t.Fatal("WindowSizeMsg must be absorbed")
	}
}

func TestNewScreenGoldenViews(t *testing.T) {
	m := scaffoldNewWithValues()
	_, _ = scaffoldUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	formView := stripScreenANSI(m.View().Content)
	for _, want := range []string{"zever new", "App name", "myapp", "esc back"} {
		if !strings.Contains(formView, want) {
			t.Fatalf("form view lacks %q:\n%s", want, formView)
		}
	}

	scaffoldNewToPreview(t, m)
	previewView := stripScreenANSI(m.View().Content)
	for _, want := range []string{"zever new", "app: myapp", "equivalent CLI: zever new myapp", "enter run"} {
		if !strings.Contains(previewView, want) {
			t.Fatalf("preview view lacks %q:\n%s", want, previewView)
		}
	}
}

func TestRunNewScreenWritesProject(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	out := filepath.Join(dir, "myapp")
	cfg := NewConfig{
		Name:       "myapp",
		OutDir:     out,
		ModulePath: "myapp",
		Batteries:  batteriesFor(nil, ""),
		Backends:   []string{"zenorm"},
	}

	summary, err := runNewScreen(cfg)
	if err != nil {
		t.Fatalf("runNewScreen: %v", err)
	}
	if !strings.Contains(summary, `"myapp"`) {
		t.Fatalf("summary = %q", summary)
	}
	if _, err := os.Stat(filepath.Join(out, "schema", "app.zen")); err != nil {
		t.Fatalf("starter schema missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "zever.yaml")); err != nil {
		t.Fatalf("zever.yaml missing: %v", err)
	}
}

func TestRunNewScreenRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	out := filepath.Join(dir, "taken")
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "existing.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := NewConfig{Name: "taken", OutDir: out, ModulePath: "taken", Batteries: batteriesFor(nil, "")}

	if _, err := runNewScreen(cfg); err == nil {
		t.Fatal("non-empty dir without force must error")
	}
}

func TestNewScreenBuildExecRunsWriter(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	m := scaffoldNewWithValues()

	ex := m.buildExec()
	if ex.CLI() != "zever new myapp" {
		t.Fatalf("exec CLI = %q", ex.CLI())
	}

	msg := ex.Start()
	done, ok := msg.(tui.ExecDoneMsg)
	if !ok {
		t.Fatalf("Start = %T, want ExecDoneMsg", msg)
	}
	if !strings.Contains(done.Output, "\"myapp\"") {
		t.Fatalf("output = %q", done.Output)
	}
}

func TestRunNewScreenFailsWithoutWorkingDir(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	// Deleting the working directory makes framework auto-detection fail
	// at Getwd, covering the resolution error path.
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}

	cfg := NewConfig{Name: "myapp", OutDir: dir + "/myapp", ModulePath: "myapp", Batteries: batteriesFor(nil, "")}
	if _, err := runNewScreen(cfg); err == nil {
		t.Fatal("missing working directory must error")
	}
}

func TestRunNewScreenFailsOnUnwritableTree(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	// A directory where the starter schema belongs makes the single
	// writer fail after the target-dir guard passes.
	out := filepath.Join(dir, "myapp")
	schemaFile := filepath.Join(out, "schema", "app.zen")
	if err := os.MkdirAll(schemaFile, 0o750); err != nil {
		t.Fatal(err)
	}

	cfg := NewConfig{
		Name:            "myapp",
		OutDir:          out,
		ModulePath:      "myapp",
		Force:           true,
		ExistingProject: true,
		Batteries:       batteriesFor(nil, ""),
	}
	if _, err := runNewScreen(cfg); err == nil {
		t.Fatal("blocked schema path must error")
	}
}
