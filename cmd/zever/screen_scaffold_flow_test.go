package main

// Shared tests for the scaffold screen plumbing: registration contract,
// inline validation, field-list parsing, CLI rendering helpers, and the
// form → preview → exec stage machine.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// scaffoldKey builds synthetic key presses: esc, enter, or a single rune.
func scaffoldKey(s string) tea.Msg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		r := []rune(s)[0]
		return tea.KeyPressMsg{Code: r, Text: s}
	}
}

// scaffoldUpdate feeds msg through m and returns the same model for chaining.
func scaffoldUpdate(t *testing.T, m tea.Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	t.Helper()

	got, cmd := m.Update(msg)

	return got, cmd
}

// scaffoldCmdMsg runs cmd and returns its message, failing on nil.
func scaffoldCmdMsg(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()

	if cmd == nil {
		t.Fatal("want non-nil cmd")
	}

	return cmd()
}

// scaffoldContains asserts sub appears in the ANSI-stripped view.
func scaffoldContains(t *testing.T, view, sub string) {
	t.Helper()

	if !strings.Contains(stripScreenANSI(view), sub) {
		t.Fatalf("view lacks %q:\n%s", sub, stripScreenANSI(view))
	}
}

func TestRegisterScaffoldScreens(t *testing.T) {
	entries := RegisterScaffoldScreens()
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}

	want := map[string]struct{ cli, screen string }{
		"new":      {"zever new", "NewScaffoldScreen"},
		"generate": {"zever generate", "NewGenerateScreen"},
		"extract":  {"zever extract", "NewExtractScreen"},
	}

	for _, e := range entries {
		w, ok := want[e.Name]
		if !ok {
			t.Fatalf("unexpected entry %q", e.Name)
		}
		if e.Group != tui.GroupScaffold {
			t.Fatalf("entry %q group = %q, want Scaffold", e.Name, e.Group)
		}
		if e.CLI != w.cli {
			t.Fatalf("entry %q CLI = %q, want %q", e.Name, e.CLI, w.cli)
		}
		if e.Screen != w.screen {
			t.Fatalf("entry %q Screen = %q, want %q", e.Name, e.Screen, w.screen)
		}
		if e.Desc == "" {
			t.Fatalf("entry %q has no description", e.Name)
		}
	}
}

func TestValidateIdentField(t *testing.T) {
	for _, ok := range []string{"shop", "Order2", "_x"} {
		if err := validateIdentField(ok); err != nil {
			t.Fatalf("validateIdentField(%q) = %v, want nil", ok, err)
		}
	}

	for _, bad := range []string{"", "has space", "9lives", "../x", "/abs", "a/b", ".", ".."} {
		if err := validateIdentField(bad); err == nil {
			t.Fatalf("validateIdentField(%q) = nil, want error", bad)
		}
	}

	if err := validateIdentField("../x"); !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("traversal error = %v, want ErrPathTraversal", err)
	}
}

func TestValidateAppNameField(t *testing.T) {
	for _, ok := range []string{"myapp", "my-app", "app_2"} {
		if err := validateAppNameField(ok); err != nil {
			t.Fatalf("validateAppNameField(%q) = %v, want nil", ok, err)
		}
	}

	for _, bad := range []string{"", "has space", "../x", "a/b", "x.y"} {
		if err := validateAppNameField(bad); err == nil {
			t.Fatalf("validateAppNameField(%q) = nil, want error", bad)
		}
	}
}

func TestValidateOptionalIdentField(t *testing.T) {
	if err := validateOptionalIdentField(""); err != nil {
		t.Fatalf("empty = %v, want nil", err)
	}
	if err := validateOptionalIdentField("  "); err != nil {
		t.Fatalf("blank = %v, want nil", err)
	}
	if err := validateOptionalIdentField("shop"); err != nil {
		t.Fatalf("shop = %v, want nil", err)
	}
	if err := validateOptionalIdentField("../x"); !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("../x = %v, want ErrPathTraversal", err)
	}
}

func TestValidatePackageNameField(t *testing.T) {
	if err := validatePackageNameField("myredis"); err != nil {
		t.Fatalf("myredis = %v, want nil", err)
	}
	for _, bad := range []string{"", "Redis", "my-redis", "../x", "9lives"} {
		if err := validatePackageNameField(bad); err == nil {
			t.Fatalf("validatePackageNameField(%q) = nil, want error", bad)
		}
	}
	if err := validatePackageNameField("../x"); !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("../x = %v, want ErrPathTraversal", err)
	}
}

func TestValidateCronField(t *testing.T) {
	if err := validateCronField("*/5 * * * *"); err != nil {
		t.Fatalf("cron = %v, want nil", err)
	}
	for _, bad := range []string{"", "   ", `0 0 * * * "x"`, `a\b`, "a\nb"} {
		if err := validateCronField(bad); err == nil {
			t.Fatalf("validateCronField(%q) = nil, want error", bad)
		}
	}
}

func TestValidateOptionalAppPath(t *testing.T) {
	if err := validateOptionalAppPath(""); err != nil {
		t.Fatalf("empty = %v, want nil", err)
	}
	if err := validateOptionalAppPath("example.com/x/internal/app"); err != nil {
		t.Fatalf("import path = %v, want nil", err)
	}
	if err := validateOptionalAppPath("a/b"); err != nil {
		t.Fatalf("relative = %v, want nil", err)
	}
	if err := validateOptionalAppPath(".."); !errors.Is(err, ErrPathTraversal) {
		t.Fatalf(".. = %v, want ErrPathTraversal", err)
	}
}

func TestParseEntityFieldList(t *testing.T) {
	got, err := parseEntityFieldList("title:string, total:int64")
	if err != nil {
		t.Fatalf("parse = %v", err)
	}
	if len(got) != 2 || got[0] != (EntityField{Name: "title", Type: "string"}) || got[1].Type != "int64" {
		t.Fatalf("parse = %+v", got)
	}

	got, err = parseEntityFieldList("  ")
	if err != nil || got != nil {
		t.Fatalf("blank = %+v, %v; want nil, nil", got, err)
	}

	if _, parseErr := parseEntityFieldList("bogus"); parseErr == nil {
		t.Fatal("bogus field must error")
	}
	if _, typeErr := parseEntityFieldList("title:nosuchtype"); typeErr == nil {
		t.Fatal("unknown scalar must error")
	}
	// Blank segments are skipped, not errors.
	got, err = parseEntityFieldList("title:string,,total:int64,")
	if err != nil || len(got) != 2 {
		t.Fatalf("sparse list = %+v, %v", got, err)
	}
	if err := validateEntityFieldList("title:string"); err != nil {
		t.Fatalf("validate = %v", err)
	}
	if err := validateEntityFieldList("bogus"); err == nil {
		t.Fatal("validate must error")
	}
}

func TestParseAdapterFieldList(t *testing.T) {
	got, err := parseAdapterFieldList("api_key:string, timeout:duration")
	if err != nil {
		t.Fatalf("parse = %v", err)
	}
	if len(got) != 2 || got[0] != (AdapterOption{Key: "api_key", Type: "string"}) {
		t.Fatalf("parse = %+v", got)
	}

	got, err = parseAdapterFieldList("")
	if err != nil || got != nil {
		t.Fatalf("empty = %+v, %v; want nil, nil", got, err)
	}

	if _, parseErr := parseAdapterFieldList("bogus"); parseErr == nil {
		t.Fatal("bogus field must error")
	}
	// Blank segments are skipped, not errors.
	got, err = parseAdapterFieldList("api_key:string,,timeout:duration")
	if err != nil || len(got) != 2 {
		t.Fatalf("sparse list = %+v, %v", got, err)
	}
	if err := validateAdapterFieldList("k:string"); err != nil {
		t.Fatalf("validate = %v", err)
	}
	if err := validateAdapterFieldList("bogus"); err == nil {
		t.Fatal("validate must error")
	}
}

func TestCLIHelpers(t *testing.T) {
	if got := cliQuote("shop"); got != "shop" {
		t.Fatalf("cliQuote(shop) = %q", got)
	}
	if got := cliQuote("my app"); got != `"my app"` {
		t.Fatalf("cliQuote(my app) = %q", got)
	}
	if got := cliQuote(""); got != `""` {
		t.Fatalf("cliQuote(empty) = %q", got)
	}
	if got := cliFlag("out", ""); got != "" {
		t.Fatalf("cliFlag empty = %q", got)
	}
	if got := cliFlag("out", "./shop-service"); got != " --out ./shop-service" {
		t.Fatalf("cliFlag = %q", got)
	}
	if got := forceFlag(false); got != "" {
		t.Fatalf("forceFlag(false) = %q", got)
	}
	if got := forceFlag(true); got != " --force" {
		t.Fatalf("forceFlag(true) = %q", got)
	}
	if got := previewRow("out", ""); got != "  out: (default)" {
		t.Fatalf("previewRow empty = %q", got)
	}
	if got := splitFieldList(""); len(got) != 0 {
		t.Fatalf("splitFieldList empty = %v", got)
	}
	if got := splitFieldList("a:string,, b:int64"); len(got) != 2 {
		t.Fatalf("splitFieldList = %v", got)
	}
	if got := nonEmptyOr("", "fb"); got != "fb" {
		t.Fatalf("nonEmptyOr = %q", got)
	}
	if got := nonEmptyOr("x", "fb"); got != "x" {
		t.Fatalf("nonEmptyOr = %q", got)
	}
	if got := backCmd(); got != (tui.BackMsg{}) {
		t.Fatalf("backCmd = %#v", got)
	}
}

func TestStripScreenANSI(t *testing.T) {
	if got := stripScreenANSI("\x1b[1mbold\x1b[0m plain"); got != "bold plain" {
		t.Fatalf("strip = %q", got)
	}
	if got := stripScreenANSI("a\ab"); got != "ab" {
		t.Fatalf("strip BEL = %q", got)
	}
}

func TestFlowIgnoresStrayCompletionMsg(t *testing.T) {
	// A completion message arriving before any exec started (wrong stage)
	// is absorbed: nothing runs, no state changes.
	m := NewScaffoldScreen()
	m.name = "myapp"
	m.flow.form = m.buildForm()
	m.freezePreview()
	m.flow.stage = stagePreview

	if _, cmd := scaffoldUpdate(t, m, tui.ExecDoneMsg{Output: "late"}); cmd != nil {
		t.Fatal("stray done msg must yield nil cmd")
	}
	if m.flow.stage != stagePreview {
		t.Fatal("stray done msg must not change stage")
	}
}

func TestFlowInitNilForm(t *testing.T) {
	var f scaffoldFlow
	if cmd := f.initFlow(); cmd != nil {
		t.Fatal("nil form Init must return nil cmd")
	}
	if cmd := f.setSize(0, 0); cmd != nil {
		t.Fatal("nil form setSize must return nil cmd")
	}
}

// scaffoldRunCmd executes cmd with a timeout so a blocking command fails
// the test instead of hanging it. Only form-stage commands may run here:
// executing an exec Start would run real cores, and executing Init would
// hit runtime requests (window size) that block without a runtime.
func scaffoldRunCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()

	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()

	select {
	case msg := <-ch:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("command blocked (timer leaked into unit test)")
		return nil
	}
}

// scaffoldPumpCmd drives huh's synchronous focus handoffs the way a real
// runtime would: execute cmd, feed the message back, and repeat once more
// (huh's own batchUpdate pattern). It never runs Init and never loops to
// quiescence, so cursor-blink timers cannot stall the suite.
func scaffoldPumpCmd(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()

	for i := 0; i < 2 && cmd != nil; i++ {
		msg := scaffoldRunCmd(t, cmd)
		if msg == nil {
			return m
		}

		var next tea.Cmd
		m, next = m.Update(msg)
		cmd = next
	}

	return m
}

// scaffoldTapEnter sends enter through m and pumps the resulting focus
// handoffs, returning the updated model.
func scaffoldTapEnter(t *testing.T, m tea.Model) tea.Model {
	t.Helper()

	var cmd tea.Cmd
	m, cmd = m.Update(scaffoldKey("enter"))

	return scaffoldPumpCmd(t, m, cmd)
}

// scaffoldWorkingDir chdirs to dir for the test, restoring on cleanup. Owned
// by the scaffold screens so churn elsewhere cannot break these tests.
func scaffoldWorkingDir(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
}

// scaffoldAssertZenParses fails when path does not re-parse as valid .zen.
func scaffoldAssertZenParses(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if _, diags := parseZen(path, data); diags.HasErrors() {
		t.Fatalf("result does not re-parse:\n%s\ndiagnostics: %v", data, diags)
	}
}

// scaffoldExtractProject lays out a minimal single-module project (go.mod +
// schema/billing/billing.zen) in a temp dir and makes it the working
// directory, returning both paths.
func scaffoldExtractProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	scaffoldWorkingDir(t, dir)

	gomod := "module example.com/shop\n\ngo 1.24\n\n" +
		"require github.com/zenta-dev/zever v0.0.0-00010101000000-000000000000\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	schema := "entity Order {\n\tid: uuid @primary\n}\n"
	path := filepath.Join(dir, "schema", "billing", "billing.zen")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(schema), 0o600); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	return dir
}
