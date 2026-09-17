package main

// Tests for GenerateScreen: kind picking, per-kind CLI/preview rendering,
// stage transitions, golden views, and every runGenerate*Screen core path in
// temp dirs.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// scaffoldGenerateWithKind picks kind and bakes valid values into its
// subform (values snapshot at construction, so they are set first).
func scaffoldGenerateWithKind(kind string) *GenerateScreen {
	m := NewGenerateScreen()
	m.kind = kind
	m.kindChosen = true

	switch kind {
	case "module":
		m.name = "shop"
	case "entity":
		m.module, m.name, m.fieldsText = "shop", "Order", "title:string"
	case "job":
		m.module, m.name = "shop", "SendEmail"
	case "schedule":
		m.module, m.name, m.cron, m.dispatch = "shop", "Nightly", "0 0 * * *", "SendEmail"
	case "tinker":
		m.appPkg, m.outDir = "example.com/x/internal/app", "cmd/tinker-shim"
	case "adapter":
		m.battery, m.adapterName = "cache", "myredis"
	}

	m.flow.form = m.buildSubform()
	m.flow.title = "zever generate " + kind

	return m
}

// scaffoldGenerateToPreview freezes preview deterministically (keyboard
// walks cover the same transition in TestGenerateSubformEscAndConfirm).
func scaffoldGenerateToPreview(m *GenerateScreen) {
	m.freezePreview()
	m.flow.stage = stagePreview
	m.flow.cli = m.generateCLI()
}

func TestGenerateKindPicker(t *testing.T) {
	m := NewGenerateScreen()
	if m.kindChosen {
		t.Fatal("kind must start unpicked")
	}
	if m.Init() == nil {
		t.Fatal("Init must arm the kind form")
	}

	var asModel tea.Model = m

	_, cmd := scaffoldUpdate(t, m, scaffoldKey("esc"))
	if msg := scaffoldCmdMsg(t, cmd); msg != (tui.BackMsg{}) {
		t.Fatalf("kind esc = %#v, want BackMsg", msg)
	}

	// Window sizing reaches both the flow and the kind form.
	_, cmd = scaffoldUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	asModel = scaffoldPumpCmd(t, asModel, cmd)
	if _, isGen := asModel.(*GenerateScreen); !isGen {
		t.Fatalf("model = %T, want *GenerateScreen", asModel)
	}

	// Default kind (entity) completes with one enter.
	asModel = scaffoldTapEnter(t, asModel)
	m, ok := asModel.(*GenerateScreen)
	if !ok {
		t.Fatalf("model = %T, want *GenerateScreen", asModel)
	}
	if !m.kindChosen || m.flow.title != "zever generate entity" {
		t.Fatalf("kindChosen = %v title = %q", m.kindChosen, m.flow.title)
	}

	scaffoldContains(t, m.View().Content, "zever generate entity")
}

func TestGenerateKindPickerStrayMsg(t *testing.T) {
	m := NewGenerateScreen()
	_, _ = scaffoldUpdate(t, m, "stray")
	if m.kindChosen {
		t.Fatal("stray msg must not pick a kind")
	}
}

func TestGenerateCLITable(t *testing.T) {
	cases := []struct {
		kind string
		cli  string
	}{
		{"module", "zever generate module shop"},
		{"entity", "zever generate entity shop Order --field title:string"},
		{"job", "zever generate job shop SendEmail --queue default"},
		{"schedule", `zever generate schedule shop Nightly --cron "0 0 * * *" --dispatch SendEmail`},
		{"server", "zever generate server"},
		{"worker", "zever generate worker"},
		{"seed", "zever generate seed"},
		{"tinker", "zever generate tinker --app example.com/x/internal/app --dir cmd/tinker-shim"},
		{"adapter", "zever generate adapter cache myredis"},
	}

	for _, c := range cases {
		m := scaffoldGenerateWithKind(c.kind)
		if got := m.generateCLI(); got != c.cli {
			t.Fatalf("kind %s CLI = %q, want %q", c.kind, got, c.cli)
		}
	}

	// Force and blank-queue variants.
	m := scaffoldGenerateWithKind("server")
	m.force = true
	if got := m.generateCLI(); got != "zever generate server --force" {
		t.Fatalf("server force CLI = %q", got)
	}

	m = scaffoldGenerateWithKind("job")
	m.queue = ""
	if got := m.generateCLI(); got != "zever generate job shop SendEmail --queue default" {
		t.Fatalf("job blank-queue CLI = %q", got)
	}

	m = scaffoldGenerateWithKind("adapter")
	m.fieldsText = "api_key:string"
	m.force = true
	if got := m.generateCLI(); got != "zever generate adapter cache myredis --field api_key:string --force" {
		t.Fatalf("adapter fields CLI = %q", got)
	}

	m = scaffoldGenerateWithKind("bogus")
	if got := m.generateCLI(); got != "zever generate" {
		t.Fatalf("unknown kind CLI = %q", got)
	}
}

func TestGeneratePreviewTable(t *testing.T) {
	for _, kind := range append(append([]string{}, generateKinds...), "bogus") {
		m := scaffoldGenerateWithKind(kind)
		lines := m.generatePreview()
		if len(lines) == 0 {
			t.Fatalf("kind %s preview is empty", kind)
		}
	}

	m := scaffoldGenerateWithKind("entity")
	joined := strings.Join(m.generatePreview(), "\n")
	for _, want := range []string{"module: shop", "entity: Order", "fields: title:string"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("entity preview lacks %q:\n%s", want, joined)
		}
	}

	m = scaffoldGenerateWithKind("job")
	m.queue = ""
	if !strings.Contains(strings.Join(m.generatePreview(), "\n"), "queue: default") {
		t.Fatal("blank queue must preview as default")
	}
}

func TestGenerateSubformEscAndConfirm(t *testing.T) {
	m := scaffoldGenerateWithKind("job")

	_, cmd := scaffoldUpdate(t, m, scaffoldKey("esc"))
	if msg := scaffoldCmdMsg(t, cmd); msg != (tui.BackMsg{}) {
		t.Fatalf("form esc = %#v, want BackMsg", msg)
	}

	// Navigation inside the subform never leaves the form stage.
	for _, key := range []string{"j", "k"} {
		_, _ = scaffoldUpdate(t, m, scaffoldKey(key))
	}
	if m.flow.stage != stageForm {
		t.Fatal("form nav must not advance stage")
	}

	// Walk the subform to preview with synthetic enters.
	var asModel tea.Model = m
	for i := 0; i < 12 && m.flow.stage == stageForm; i++ {
		asModel = scaffoldTapEnter(t, asModel)
		var ok bool
		m, ok = asModel.(*GenerateScreen)
		if !ok {
			t.Fatalf("model = %T, want *GenerateScreen", asModel)
		}
	}
	if m.flow.stage != stagePreview {
		t.Fatalf("stage = %d, want preview", m.flow.stage)
	}
	if m.flow.cli != "zever generate job shop SendEmail --queue default" {
		t.Fatalf("CLI = %q", m.flow.cli)
	}

	_, cmd = scaffoldUpdate(t, m, scaffoldKey("enter"))
	if cmd == nil || m.flow.stage != stageExec {
		t.Fatal("preview enter must start exec")
	}

	_, _ = scaffoldUpdate(t, m, tui.ExecDoneMsg{Output: "appended job"})
	scaffoldContains(t, m.View().Content, "equivalent CLI: zever generate job shop SendEmail --queue default")
}

func TestGenerateUnknownKindExecErrors(t *testing.T) {
	m := scaffoldGenerateWithKind("bogus")
	scaffoldGenerateToPreview(m)

	ex := m.buildExec()
	msg := ex.Start()
	got, ok := msg.(tui.ExecErrMsg)
	if !ok {
		t.Fatalf("Start = %T, want ExecErrMsg", msg)
	}
	if !strings.Contains(got.Err.Error(), "unknown kind") {
		t.Fatalf("err = %v", got.Err)
	}
}

func TestGenerateGoldenViews(t *testing.T) {
	m := NewGenerateScreen()
	_, _ = scaffoldUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	scaffoldContains(t, m.View().Content, "What to generate")

	m2 := scaffoldGenerateWithKind("schedule")
	_, _ = scaffoldUpdate(t, m2, tea.WindowSizeMsg{Width: 80, Height: 24})
	scaffoldContains(t, m2.View().Content, "zever generate schedule")
	scaffoldContains(t, m2.View().Content, "Nightly")

	scaffoldGenerateToPreview(m2)
	preview := stripScreenANSI(m2.View().Content)
	for _, want := range []string{"cron: 0 0 * * *", "dispatch: SendEmail", "equivalent CLI:"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("preview lacks %q:\n%s", want, preview)
		}
	}
}

// scaffoldGoMod writes a minimal go.mod so goModulePath resolves in temp dirs.
func scaffoldGoMod(t *testing.T, dir string) {
	t.Helper()

	content := "module example.com/shop\n\ngo 1.24\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}

func TestRunGenerateModuleScreen(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	path, err := runGenerateModuleScreen("shop")
	if err != nil {
		t.Fatalf("module: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stub missing: %v", err)
	}

	if _, err := runGenerateModuleScreen(""); err == nil {
		t.Fatal("empty name must error")
	}
}

func TestRunGenerateEntityScreen(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	if _, err := runGenerateModuleScreen("shop"); err != nil {
		t.Fatalf("module: %v", err)
	}
	path, err := runGenerateEntityScreen("shop", "Order", "title:string,total:int64")
	if err != nil {
		t.Fatalf("entity: %v", err)
	}
	scaffoldAssertZenParses(t, path)

	if _, err := runGenerateEntityScreen("shop", "Bad", "bogus"); err == nil {
		t.Fatal("bad fields must error")
	}
}

func TestRunGenerateJobAndScheduleScreens(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	if _, err := runGenerateModuleScreen("shop"); err != nil {
		t.Fatalf("module: %v", err)
	}
	if _, err := runGenerateJobScreen("shop", "SendEmail", ""); err != nil {
		t.Fatalf("job: %v", err)
	}
	if _, err := runGenerateScheduleScreen("shop", "Nightly", "0 0 * * *", "SendEmail"); err != nil {
		t.Fatalf("schedule: %v", err)
	}

	if _, err := runGenerateJobScreen("shop", "Bad job", "default"); err == nil {
		t.Fatal("bad job name must error")
	}
	if _, err := runGenerateScheduleScreen("shop", "Nope", "0 0 * * *", "Missing"); err == nil {
		t.Fatal("unknown dispatch must error")
	}
}

func TestRunGenerateServerWorkerSeedScreens(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)
	scaffoldGoMod(t, dir)

	res, err := runGenerateServerScreen(false)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	if !strings.Contains(res, "cmd/server") {
		t.Fatalf("server summary = %q", res)
	}

	res, err = runGenerateWorkerScreen(false)
	if err != nil {
		t.Fatalf("worker: %v", err)
	}
	if !strings.Contains(res, "cmd/worker") {
		t.Fatalf("worker summary = %q", res)
	}

	res, err = runGenerateSeedScreen(false)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !strings.Contains(res, "seed") {
		t.Fatalf("seed summary = %q", res)
	}
}

func TestRunGenerateServerScreenNoGoMod(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	if _, err := runGenerateServerScreen(false); err == nil {
		t.Fatal("missing go.mod must error")
	}
	if _, err := runGenerateWorkerScreen(false); err == nil {
		t.Fatal("missing go.mod must error")
	}
	if _, err := runGenerateSeedScreen(false); err == nil {
		t.Fatal("missing go.mod must error")
	}
}

func TestRunGenerateTinkerScreen(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)
	scaffoldGoMod(t, dir)

	path, err := runGenerateTinkerScreen("", "", false)
	if err != nil {
		t.Fatalf("tinker defaults: %v", err)
	}
	if !strings.Contains(path, "main.go") {
		t.Fatalf("tinker path = %q", path)
	}

	if _, err := runGenerateTinkerScreen("", "", false); err == nil {
		t.Fatal("second run without force must error")
	}
	if _, err := runGenerateTinkerScreen("example.com/shop/internal/app", "cmd/tinker-shim", true); err != nil {
		t.Fatalf("tinker force: %v", err)
	}
}

func TestRunGenerateTinkerScreenNoGoMod(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	if _, err := runGenerateTinkerScreen("", "", false); err == nil {
		t.Fatal("missing go.mod with no --app must error")
	}
}

func TestRunGenerateAdapterScreen(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	res, err := runGenerateAdapterScreen("cache", "myredis", "api_key:string", false)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	if !strings.Contains(res, "myredis") {
		t.Fatalf("adapter summary = %q", res)
	}

	if _, err := runGenerateAdapterScreen("nosuchbattery", "x", "", false); err == nil {
		t.Fatal("unknown battery must error")
	}
	if _, err := runGenerateAdapterScreen("cache", "x", "bogus", false); err == nil {
		t.Fatal("bad fields must error")
	}
}

func TestBuildExecAllKinds(t *testing.T) {
	for _, kind := range append(append([]string{}, generateKinds...), "bogus") {
		m := scaffoldGenerateWithKind(kind)
		scaffoldGenerateToPreview(m)

		ex := m.buildExec()
		if ex.CLI() == "" {
			t.Fatalf("kind %s exec CLI is empty", kind)
		}
		if !strings.HasPrefix(ex.Title(), "zever generate") {
			t.Fatalf("kind %s exec title = %q", kind, ex.Title())
		}
	}
}

// TestBuildExecStartsRunCores executes every kind's ExecFunc in a prepared
// temp workspace, covering the run closures end to end.
func TestBuildExecStartsRunCores(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)
	scaffoldGoMod(t, dir)

	for _, kind := range generateKinds {
		m := scaffoldGenerateWithKind(kind)
		ex := m.buildExec()

		msg := ex.Start()
		if done, ok := msg.(tui.ExecDoneMsg); !ok {
			t.Fatalf("kind %s Start = %T (%v), want ExecDoneMsg", kind, msg, msg)
		} else if done.Output == "" {
			t.Fatalf("kind %s output is empty", kind)
		}
	}

	m := scaffoldGenerateWithKind("bogus")
	if _, ok := m.buildExec().Start().(tui.ExecErrMsg); !ok {
		t.Fatal("bogus kind Start must fail")
	}
}

// scaffoldBrokenConfig writes an undecodable zever.yaml so loadProjectConfig
// fails, covering the resolution error returns.
func scaffoldBrokenConfig(t *testing.T) {
	t.Helper()

	if err := os.WriteFile("zever.yaml", []byte(": : :\n"), 0o600); err != nil {
		t.Fatalf("write broken config: %v", err)
	}
}

func TestRunGenerateScreensRefuseBrokenConfig(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)
	scaffoldGoMod(t, dir)
	scaffoldBrokenConfig(t)

	if _, err := runGenerateServerScreen(false); err == nil {
		t.Fatal("broken config must error for server")
	}
	if _, err := runGenerateWorkerScreen(false); err == nil {
		t.Fatal("broken config must error for worker")
	}
	if _, err := runGenerateSeedScreen(false); err == nil {
		t.Fatal("broken config must error for seed")
	}
	if _, err := runGenerateTinkerScreen("example.com/shop/internal/app", "", false); err == nil {
		t.Fatal("broken config must error for tinker defaults")
	}
}

func TestRunGenerateScreensRefuseCollisions(t *testing.T) {
	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)
	scaffoldGoMod(t, dir)

	if _, err := runGenerateServerScreen(false); err != nil {
		t.Fatalf("server: %v", err)
	}
	if _, err := runGenerateServerScreen(false); err == nil {
		t.Fatal("second server run without force must error")
	}

	if _, err := runGenerateWorkerScreen(false); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if _, err := runGenerateWorkerScreen(false); err == nil {
		t.Fatal("second worker run without force must error")
	}

	if _, err := runGenerateSeedScreen(false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := runGenerateSeedScreen(false); err == nil {
		t.Fatal("second seed run without force must error")
	}
}
