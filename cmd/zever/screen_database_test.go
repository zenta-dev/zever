package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// databaseKeyPress builds key messages the way the inspect screens do:
// esc/enter by code, runes with text so huh fields and the shared keymap
// both recognize them.
func databaseKeyPress(s string) tea.Msg {
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

// databaseTestSchema is a minimal valid schema for round-trip tests.
// Self-contained: it must not reference other agents' test fixtures.
const databaseTestSchema = `entity User {
	id: uuid @primary
	email: string @unique
}
`

// writeDatabaseFixture writes content under dir and returns its path.
func writeDatabaseFixture(t *testing.T, dir, name, content string) string {
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

// databaseSQLiteObjects returns table/index names in a sqlite file.
func databaseSQLiteObjects(t *testing.T, path string) map[string]bool {
	t.Helper()

	ensureZeverDBAdapters()

	conn, err := openZeverDB("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	defer func() { _ = conn.Close(t.Context()) }()

	rows, err := conn.Query(t.Context(), "SELECT name FROM sqlite_master WHERE type IN ('table','index')")
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	defer func() { _ = rows.Close() }()

	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}

		out[name] = true
	}

	return out
}

// stubDatabaseInteractive resets the interactive globals so core flag paths
// never prompt under test.
func stubDatabaseInteractive(t *testing.T) {
	t.Helper()

	prev := interactiveMode
	interactiveMode = false
	t.Cleanup(func() { interactiveMode = prev })
	t.Setenv("ZEVER_INTERACTIVE", "")
}

// --- RegisterDatabaseScreens contract ---

func TestRegisterDatabaseScreens(t *testing.T) {
	entries := RegisterDatabaseScreens()
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3: %+v", len(entries), entries)
	}

	byName := map[string]tui.Entry{}
	for _, e := range entries {
		if e.Group != tui.GroupDatabase {
			t.Fatalf("entry %q group = %q, want %q", e.Name, e.Group, tui.GroupDatabase)
		}
		if e.Name == "" || e.Desc == "" || e.CLI == "" || e.Screen == "" {
			t.Fatalf("entry %+v has empty field", e)
		}

		byName[e.Name] = e
	}

	for _, want := range []string{"migrate", "rollback", "seed"} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("missing entry %q in %+v", want, entries)
		}
	}

	if !strings.HasPrefix(byName["migrate"].CLI, "zever db migrate") {
		t.Fatalf("migrate CLI = %q", byName["migrate"].CLI)
	}
	if !strings.HasPrefix(byName["rollback"].CLI, "zever db rollback") {
		t.Fatalf("rollback CLI = %q", byName["rollback"].CLI)
	}
	if !strings.HasPrefix(byName["seed"].CLI, "zever db seed") {
		t.Fatalf("seed CLI = %q", byName["seed"].CLI)
	}
}

func TestDatabaseAdaptersExact(t *testing.T) {
	if len(databaseAdapters) != 2 || databaseAdapters[0] != "sqlite" || databaseAdapters[1] != "postgres" {
		t.Fatalf("adapters = %v, want exactly [sqlite postgres]", databaseAdapters)
	}

	if !isDatabaseAdapter("sqlite") || !isDatabaseAdapter("postgres") {
		t.Fatal("sqlite and postgres must validate")
	}
	if isDatabaseAdapter("mysql") || isDatabaseAdapter("") || isDatabaseAdapter("oracle") {
		t.Fatal("mysql/empty/oracle must not validate (no registered adapter)")
	}
}

// --- CLI builders: exact strings, redacted display ---

func TestBuildDatabaseMigrateCLI(t *testing.T) {
	files := []string{"schema/app.zen", "schema/shop.zen"}

	full := buildDatabaseMigrateCLI("sqlite", "data/app.db", files, false, false, false)
	want := "zever db migrate --adapter sqlite --dsn data/app.db schema/app.zen schema/shop.zen"
	if full != want {
		t.Fatalf("cli = %q, want %q", full, want)
	}

	flags := buildDatabaseMigrateCLI("postgres", "postgres://localhost/db", files, true, true, false)
	for _, part := range []string{"--dry-run", "--drop-columns", "--adapter postgres"} {
		if !strings.Contains(flags, part) {
			t.Fatalf("cli missing %q: %q", part, flags)
		}
	}

	redacted := buildDatabaseMigrateCLI("sqlite", "data/app.db", files, true, true, true)
	if strings.Contains(redacted, "data/app.db") {
		t.Fatalf("redacted CLI leaks DSN: %q", redacted)
	}
	if !strings.Contains(redacted, "--dsn ***") {
		t.Fatalf("redacted CLI must show masked dsn: %q", redacted)
	}

	bare := buildDatabaseMigrateCLI("", "", nil, false, false, true)
	if bare != "zever db migrate" {
		t.Fatalf("bare cli = %q", bare)
	}
}

func TestBuildDatabaseRollbackCLI(t *testing.T) {
	got := buildDatabaseRollbackCLI("sqlite", "data/app.db", 3, false, false)
	want := "zever db rollback --adapter sqlite --dsn data/app.db -n 3"
	if got != want {
		t.Fatalf("cli = %q, want %q", got, want)
	}

	dry := buildDatabaseRollbackCLI("postgres", "pg", 1, true, false)
	if !strings.Contains(dry, "--dry-run") {
		t.Fatalf("dry cli = %q", dry)
	}

	redacted := buildDatabaseRollbackCLI("sqlite", "s3cr3t", 1, false, true)
	if strings.Contains(redacted, "s3cr3t") || !strings.Contains(redacted, "--dsn ***") {
		t.Fatalf("redacted cli = %q", redacted)
	}
}

func TestBuildDatabaseSeedCLI(t *testing.T) {
	if got := buildDatabaseSeedCLI(nil); got != "zever db seed" {
		t.Fatalf("cli = %q", got)
	}

	got := buildDatabaseSeedCLI([]string{"--only=users", "extra arg"})
	want := "zever db seed --only=users \"extra arg\""
	if got != want {
		t.Fatalf("cli = %q, want %q", got, want)
	}
}

func TestSplitDatabaseFileList(t *testing.T) {
	got := splitDatabaseFileList("a.zen, b.zen  c.zen\nd.zen;e.zen")
	want := []string{"a.zen", "b.zen", "c.zen", "d.zen", "e.zen"}
	if len(got) != len(want) {
		t.Fatalf("split = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("split = %v, want %v", got, want)
		}
	}
	if len(splitDatabaseFileList("  ")) != 0 {
		t.Fatal("blank input must yield no files")
	}
}

func TestSanitizeDatabaseOutput(t *testing.T) {
	out := sanitizeDatabaseOutput("open data/app.db: boom data/app.db", "data/app.db")
	if strings.Contains(out, "data/app.db") {
		t.Fatalf("leak: %q", out)
	}
	if !strings.Contains(out, "***") {
		t.Fatalf("missing mask: %q", out)
	}
	if got := sanitizeDatabaseOutput("clean", "", "  "); got != "clean" {
		t.Fatalf("empty secrets must be skipped: %q", got)
	}
}

func TestCaptureDatabaseOutput(t *testing.T) {
	out, err := captureDatabaseOutput(func() error { return nil })
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if out != "" {
		t.Fatalf("empty fn must capture empty, got %q", out)
	}

	boom := errors.New("boom")
	out, err = captureDatabaseOutput(func() error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if out != "" {
		t.Fatalf("out = %q", out)
	}
}

func TestCaptureDatabaseOutputPipeFailure(t *testing.T) {
	prev := databasePipe
	databasePipe = func() (*os.File, *os.File, error) { return nil, nil, errors.New("no pipe") }
	t.Cleanup(func() { databasePipe = prev })

	if _, err := captureDatabaseOutput(func() error { return nil }); err == nil {
		t.Fatal("pipe failure must surface")
	}
}

// databasePing exercises the Update default branch (non-key,
// non-exec messages) without depending on other agents' message types.
type databasePing struct{}

// --- migrate screen ---

func TestMigrateScreenInitAndGolden(t *testing.T) {
	m := NewMigrateScreen()
	if m.Init() == nil {
		t.Fatal("Init must arm the form")
	}

	v := stripScreenANSI(m.View().Content)
	for _, want := range []string{"migrate", "zever db migrate", "Adapter", "DSN", "Schema files", "DROP COLUMN", "esc back"} {
		if !strings.Contains(v, want) {
			t.Fatalf("form view missing %q:\n%s", want, v)
		}
	}
	if got := m.CLI(); !strings.HasPrefix(got, "zever db migrate --adapter sqlite") {
		t.Fatalf("default CLI = %q", got)
	}
}

func TestMigrateScreenEscCancels(t *testing.T) {
	m := NewMigrateScreen()
	_, cmd := m.Update(databaseKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc in form must yield a cmd")
	}
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}
	if !m.canceled {
		t.Fatal("esc must mark canceled")
	}
}

func TestMigrateScreenSubmitRequiresFiles(t *testing.T) {
	m := NewMigrateScreen()
	m.adapter = "sqlite"
	m.filesRaw = "   "
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if ms.stage != databaseStageForm {
		t.Fatal("missing files must stay in form")
	}
	if !errors.Is(ms.err, errDatabaseNoFiles) {
		t.Fatalf("err = %v, want errDatabaseNoFiles", ms.err)
	}
	if v := stripScreenANSI(ms.View().Content); !strings.Contains(v, "at least one schema file") {
		t.Fatalf("view must show the error:\n%s", v)
	}
}

func TestMigrateScreenSubmitRejectsAdapter(t *testing.T) {
	m := NewMigrateScreen()
	m.adapter = "mysql"
	m.filesRaw = "schema/app.zen"
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if ms.stage != databaseStageForm || !errors.Is(ms.err, errDatabaseBadAdapter) {
		t.Fatalf("stage = %v err = %v, want form + bad adapter", ms.stage, ms.err)
	}
}

func TestMigrateScreenSubmitStartsPreview(t *testing.T) {
	m := NewMigrateScreen()
	m.adapter = "sqlite"
	m.dsn = "data/app.db"
	m.filesRaw = "schema/app.zen"
	m.form.State = huh.StateCompleted

	got, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if ms.stage != databaseStagePreview {
		t.Fatalf("stage = %v, want preview", ms.stage)
	}
	if cmd == nil {
		t.Fatal("submit must start the dry-run exec")
	}
	if !ms.hasExec {
		t.Fatal("preview exec must be armed")
	}
	cli := ms.exec.CLI()
	if strings.Contains(cli, "data/app.db") {
		t.Fatalf("exec CLI leaks DSN: %q", cli)
	}
	if !strings.Contains(cli, "--dry-run") || !strings.Contains(cli, "zever db migrate") {
		t.Fatalf("exec CLI = %q", cli)
	}
}

func TestMigrateScreenPreviewDoneDryRunOnly(t *testing.T) {
	m := NewMigrateScreen()
	m.adapter = "sqlite"
	m.dsn = "data/app.db"
	m.filesRaw = "schema/app.zen"
	m.dryRun = true
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}

	done, _ := ms.Update(tui.ExecDoneMsg{Output: "CREATE TABLE x"})
	ms, ok = done.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", done)
	}
	if ms.stage != databaseStageConfirm {
		t.Fatalf("stage = %v, want confirm", ms.stage)
	}

	v := stripScreenANSI(ms.View().Content)
	for _, want := range []string{"CREATE TABLE x", "zever db migrate", "no changes applied", "enter back"} {
		if !strings.Contains(v, want) {
			t.Fatalf("confirm view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "data/app.db") {
		t.Fatalf("confirm view leaks DSN:\n%s", v)
	}

	// Dry-run only: enter goes back to the form, esc cancels out.
	back, cmd := ms.Update(databaseKeyPress("enter"))
	backScreen, ok := back.(*MigrateScreen)
	if !ok {
		t.Fatalf("enter-back = %T, want *MigrateScreen", back)
	}
	if backScreen.stage != databaseStageForm {
		t.Fatal("enter after dry-run must return to form")
	}
	if cmd != nil {
		t.Fatal("enter-back must not start work")
	}

	ms.stage = databaseStageConfirm
	_, cancel := ms.Update(databaseKeyPress("esc"))
	if _, ok := cancel().(tui.CanceledMsg); !ok {
		t.Fatalf("esc = %T, want CanceledMsg", cancel())
	}
}

func TestMigrateScreenApplyFlow(t *testing.T) {
	m := NewMigrateScreen()
	m.adapter = "sqlite"
	m.dsn = "data/app.db"
	m.filesRaw = "schema/app.zen"
	m.dryRun = false
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}

	done, _ := ms.Update(tui.ExecDoneMsg{Output: "plan"})
	ms, ok = done.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", done)
	}

	v := stripScreenANSI(ms.View().Content)
	if !strings.Contains(v, "Apply these statements?") {
		t.Fatalf("confirm must offer apply:\n%s", v)
	}

	applied, cmd := ms.Update(databaseKeyPress("enter"))
	ms, ok = applied.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", applied)
	}
	if ms.stage != databaseStageExec {
		t.Fatalf("stage = %v, want exec", ms.stage)
	}
	if cmd == nil {
		t.Fatal("apply confirm must start the apply exec")
	}
	if strings.Contains(ms.exec.CLI(), "--dry-run") {
		t.Fatalf("apply CLI must not be a dry run: %q", ms.exec.CLI())
	}

	finished, _ := ms.Update(tui.ExecDoneMsg{Output: "applied 1 statement(s)"})
	finishedMs, ok := finished.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", finished)
	}
	v = stripScreenANSI(finishedMs.View().Content)
	if !strings.Contains(v, "applied 1 statement(s)") || !strings.Contains(v, "zever db migrate") {
		t.Fatalf("apply view wrong:\n%s", v)
	}

	// esc after completion backs out.
	finishedMs2, ok := finished.(*MigrateScreen)
	if !ok {
		t.Fatalf("finished = %T, want *MigrateScreen", finished)
	}
	_, cancel := finishedMs2.Update(databaseKeyPress("esc"))
	if _, ok := cancel().(tui.CanceledMsg); !ok {
		t.Fatalf("esc = %T, want CanceledMsg", cancel())
	}
}

func TestMigrateScreenPreviewEscWhileRunning(t *testing.T) {
	m := NewMigrateScreen()
	m.filesRaw = "schema/app.zen"
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}

	_, cmd := ms.Update(databaseKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc while preview runs must yield a cmd")
	}
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}
}

func TestMigrateScreenExecErrorRedactsDSN(t *testing.T) {
	m := NewMigrateScreen()
	m.dsn = "s3cr3t-dsn"
	m.filesRaw = "schema/app.zen"
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}

	failed, _ := ms.Update(tui.ExecErrMsg{Err: errors.New("open s3cr3t-dsn: refused")})
	ms, ok = failed.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", failed)
	}
	if ms.stage != databaseStageConfirm {
		t.Fatalf("failed preview must land on confirm, got %v", ms.stage)
	}
	if v := stripScreenANSI(ms.View().Content); strings.Contains(v, "s3cr3t-dsn") {
		t.Fatalf("error view leaks DSN:\n%s", v)
	}
}

func TestMigrateScreenFormAbortedCancels(t *testing.T) {
	m := NewMigrateScreen()
	m.form.State = huh.StateAborted

	got, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}
	gotScreen, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if !gotScreen.canceled {
		t.Fatal("aborted form must mark canceled")
	}
}

func TestMigrateScreenWindowSizeForwards(t *testing.T) {
	m := NewMigrateScreen()
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if ms.stage != databaseStageForm || ms.err != nil || ms.canceled {
		t.Fatalf("resize must stay in form quietly: %+v", ms)
	}
}

func TestMigrateScreenFormKeypressForwards(t *testing.T) {
	m := NewMigrateScreen()
	got, _ := m.Update(databaseKeyPress("x"))
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if ms.stage != databaseStageForm {
		t.Fatalf("typing must stay in form: %v", ms.stage)
	}
}

func TestMigrateScreenDefaultBranchNoop(t *testing.T) {
	m := NewMigrateScreen()
	got, cmd := m.Update(databasePing{})
	pingScreen, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if pingScreen.stage != databaseStageForm || cmd != nil {
		t.Fatal("unknown msg in form must forward quietly")
	}

	m.stage = databaseStageConfirm
	_, cmd = m.Update(databasePing{})
	if cmd != nil {
		t.Fatal("unknown msg outside form must be a no-op")
	}
}

func TestMigrateScreenInvalidStageNoop(t *testing.T) {
	m := NewMigrateScreen()
	m.stage = databaseStage(99)

	got, cmd := m.Update(databaseKeyPress("x"))
	invalidScreen, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if invalidScreen.stage != databaseStage(99) || cmd != nil {
		t.Fatal("unknown stage must be a no-op")
	}
}

func TestMigrateScreenEmptyAdapterDefaults(t *testing.T) {
	m := NewMigrateScreen()
	m.adapter = ""
	m.filesRaw = "schema/app.zen"
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if ms.stage != databaseStagePreview {
		t.Fatalf("stage = %v, want preview", ms.stage)
	}
	if !strings.Contains(ms.CLI(), "--adapter sqlite") {
		t.Fatalf("CLI = %q, want sqlite default", ms.CLI())
	}
}

func TestMigrateScreenDropColumnsFlag(t *testing.T) {
	m := NewMigrateScreen()
	m.filesRaw = "schema/app.zen"
	m.dropColumns = true
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if !strings.Contains(ms.exec.CLI(), "--drop-columns") {
		t.Fatalf("preview CLI = %q, want drop-columns", ms.exec.CLI())
	}
}

func TestMigrateScreenPreviewKeypressCompletes(t *testing.T) {
	m := NewMigrateScreen()
	m.filesRaw = "schema/app.zen"
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}

	// The exec finished (done message applied directly), but a spinner
	// tick arrives first: the preview stage must still advance.
	updated, _ := ms.exec.Update(tui.ExecDoneMsg{Output: "plan"})
	updatedExec, ok := updated.(tui.ExecModel)
	if !ok {
		t.Fatalf("Update = %T, want tui.ExecModel", updated)
	}
	ms.exec = updatedExec

	moved, _ := ms.Update(databaseKeyPress("x"))
	movedScreen, ok := moved.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", moved)
	}
	if movedScreen.stage != databaseStageConfirm {
		t.Fatal("finished preview must advance on any key")
	}
}

func TestMigrateScreenConfirmIgnoresOtherKeys(t *testing.T) {
	m := NewMigrateScreen()
	m.filesRaw = "schema/app.zen"
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	done, _ := ms.Update(tui.ExecDoneMsg{Output: "plan"})
	ms, ok = done.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", done)
	}

	kept, cmd := ms.Update(databaseKeyPress("x"))
	keptScreen, ok := kept.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", kept)
	}
	if keptScreen.stage != databaseStageConfirm || cmd != nil {
		t.Fatal("non-enter key in confirm must be a no-op")
	}
}

func TestMigrateScreenExecKeysDelegate(t *testing.T) {
	m := NewMigrateScreen()
	m.filesRaw = "schema/app.zen"
	m.dryRun = false
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	done, _ := ms.Update(tui.ExecDoneMsg{Output: "plan"})
	ms, ok = done.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", done)
	}
	applied, _ := ms.Update(databaseKeyPress("enter"))
	ms, ok = applied.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", applied)
	}

	// Running apply: esc delegates into the kit model (in-flight cancel).
	_, cmd := ms.Update(databaseKeyPress("esc"))
	if cmd == nil {
		t.Fatal("esc while applying must yield the kit cancel cmd")
	}
	if _, cancelOK := cmd().(tui.CanceledMsg); !cancelOK {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}

	// Settled apply: other keys delegate quietly.
	finished, _ := ms.Update(tui.ExecDoneMsg{Output: "ok"})
	finishedMs, ok := finished.(*MigrateScreen)
	if !ok {
		t.Fatalf("finished = %T, want *MigrateScreen", finished)
	}
	kept, cmd := finishedMs.Update(databaseKeyPress("x"))
	keptMs, ok := kept.(*MigrateScreen)
	if !ok {
		t.Fatalf("kept = %T, want *MigrateScreen", kept)
	}
	if keptMs.stage != databaseStageExec || cmd != nil {
		t.Fatal("settled exec must ignore other keys")
	}
}

func TestMigrateScreenExecWithoutArmedExec(t *testing.T) {
	m := NewMigrateScreen()
	got, cmd := m.Update(tui.ExecDoneMsg{Output: "stray"})
	strayScreen, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	if strayScreen.stage != databaseStageForm || cmd != nil {
		t.Fatal("stray exec msg with no exec must be a no-op")
	}
}

func TestMigrateScreenApplyRequiresDSN(t *testing.T) {
	stubDatabaseInteractive(t)

	dir := t.TempDir()
	schemaPath := writeDatabaseFixture(t, dir, "schema.zen", databaseTestSchema)

	m := NewMigrateScreen()
	m.dsn = ""
	m.filesRaw = schemaPath
	m.dryRun = false
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}
	done, _ := ms.Update(tui.ExecDoneMsg{Output: "plan"})
	ms, ok = done.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", done)
	}
	applied, _ := ms.Update(databaseKeyPress("enter"))
	ms, ok = applied.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", applied)
	}

	msg := ms.exec.Start()
	errMsg, ok := msg.(tui.ExecErrMsg)
	if !ok {
		t.Fatalf("dsn-less apply = %T, want ExecErrMsg", msg)
	}
	if !strings.Contains(errMsg.Err.Error(), "--dsn is required") {
		t.Fatalf("err = %v", errMsg.Err)
	}

	failed, _ := ms.Update(msg)
	failedScreen, ok := failed.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", failed)
	}
	if failedScreen.stage != databaseStageExec {
		t.Fatal("failed apply must stay in exec")
	}
}

func TestMigrateScreenSQLiteRoundTrip(t *testing.T) {
	stubDatabaseInteractive(t)

	dir := t.TempDir()
	schemaPath := writeDatabaseFixture(t, dir, "schema.zen", databaseTestSchema)
	dbPath := filepath.Join(dir, "app.db")

	m := NewMigrateScreen()
	m.adapter = "sqlite"
	m.dsn = dbPath
	m.filesRaw = schemaPath
	m.dryRun = false
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ms, ok := got.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", got)
	}

	// Step 1: dry-run preview plans without executing.
	preview := ms.exec.Start()
	plan, ok := preview.(tui.ExecDoneMsg)
	if !ok {
		t.Fatalf("preview = %T (%v), want ExecDoneMsg", preview, preview)
	}
	if !strings.Contains(plan.Output, `CREATE TABLE IF NOT EXISTS "users"`) {
		t.Fatalf("plan missing DDL:\n%s", plan.Output)
	}
	if strings.Contains(plan.Output, dbPath) {
		t.Fatalf("plan leaks DSN path:\n%s", plan.Output)
	}
	if objs := databaseSQLiteObjects(t, dbPath); objs["users"] {
		t.Fatalf("dry-run must not create tables: %v", objs)
	}

	// Step 2: confirm applies for real.
	done, _ := ms.Update(tui.ExecDoneMsg{Output: plan.Output})
	ms, ok = done.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", done)
	}
	applied, _ := ms.Update(databaseKeyPress("enter"))
	ms, ok = applied.(*MigrateScreen)
	if !ok {
		t.Fatalf("Update = %T, want *MigrateScreen", applied)
	}

	msg := ms.exec.Start()
	if _, ok := msg.(tui.ExecDoneMsg); !ok {
		t.Fatalf("apply = %T (%v), want ExecDoneMsg", msg, msg)
	}
	if objs := databaseSQLiteObjects(t, dbPath); !objs["users"] {
		t.Fatalf("apply must create tables: %v", objs)
	}
}

// --- rollback screen ---

func TestRollbackScreenInitAndGolden(t *testing.T) {
	m := NewRollbackScreen()
	if m.Init() == nil {
		t.Fatal("Init must arm the form")
	}

	v := stripScreenANSI(m.View().Content)
	for _, want := range []string{"rollback", "zever db rollback", "DANGER", "unrecoverable", "-n 1", "esc back"} {
		if !strings.Contains(v, want) {
			t.Fatalf("form view missing %q:\n%s", want, v)
		}
	}
	if got := m.CLI(); got != "zever db rollback --adapter sqlite -n 1" {
		t.Fatalf("default CLI = %q", got)
	}
}

func TestRollbackScreenEscCancels(t *testing.T) {
	m := NewRollbackScreen()
	_, cmd := m.Update(databaseKeyPress("esc"))
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}
	if !m.canceled {
		t.Fatal("esc must mark canceled")
	}
}

func TestRollbackScreenDangerRefuse(t *testing.T) {
	m := NewRollbackScreen()
	m.adapter = "sqlite"
	m.dsn = "data/app.db"
	m.countRaw = "2"
	m.confirmed = false
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rs, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}
	if rs.stage != databaseStageForm {
		t.Fatal("refused confirm must stay in form")
	}
	if !errors.Is(rs.err, errDatabaseConfirmRequired) {
		t.Fatalf("err = %v, want confirm required", rs.err)
	}

	v := stripScreenANSI(rs.View().Content)
	for _, want := range []string{"confirm", "DANGER"} {
		if !strings.Contains(v, want) {
			t.Fatalf("refusal view missing %q:\n%s", want, v)
		}
	}
}

func TestRollbackScreenBadCounts(t *testing.T) {
	for _, raw := range []string{"0", "-2", "abc", "  "} {
		m := NewRollbackScreen()
		m.adapter = "sqlite"
		m.dsn = "data/app.db"
		m.countRaw = raw
		m.confirmed = true
		m.form.State = huh.StateCompleted

		got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		rs, ok := got.(*RollbackScreen)
		if !ok {
			t.Fatalf("Update = %T, want *RollbackScreen", got)
		}
		if !errors.Is(rs.err, errDatabaseBadCount) {
			t.Fatalf("count %q: err = %v, want bad count", raw, rs.err)
		}
	}
}

func TestRollbackScreenMissingDSN(t *testing.T) {
	m := NewRollbackScreen()
	m.countRaw = "1"
	m.confirmed = true
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rs, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}
	if !errors.Is(rs.err, errDatabaseDSNRequired) {
		t.Fatalf("err = %v, want dsn required", rs.err)
	}
}

func TestRollbackScreenRejectsAdapter(t *testing.T) {
	for _, adapter := range []string{"mysql", "oracle"} {
		m := NewRollbackScreen()
		m.adapter = adapter
		m.dsn = "x"
		m.countRaw = "1"
		m.confirmed = true
		m.form.State = huh.StateCompleted

		got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		rs, ok := got.(*RollbackScreen)
		if !ok {
			t.Fatalf("Update = %T, want *RollbackScreen", got)
		}
		if !errors.Is(rs.err, errDatabaseBadAdapter) {
			t.Fatalf("adapter %q: err = %v, want bad adapter", adapter, rs.err)
		}
	}
}

func TestRollbackScreenAcceptStartsExec(t *testing.T) {
	m := NewRollbackScreen()
	m.adapter = "sqlite"
	m.dsn = "data/app.db"
	m.countRaw = "2"
	m.confirmed = true
	m.form.State = huh.StateCompleted

	got, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rs, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}
	if rs.stage != databaseStageExec {
		t.Fatalf("stage = %v, want exec", rs.stage)
	}
	if cmd == nil {
		t.Fatal("accept must start the exec")
	}
	if cli := rs.exec.CLI(); cli != "zever db rollback --adapter sqlite --dsn *** -n 2" {
		t.Fatalf("exec CLI = %q", cli)
	}

	v := stripScreenANSI(rs.View().Content)
	if strings.Contains(v, "data/app.db") {
		t.Fatalf("exec view leaks DSN:\n%s", v)
	}

	finished, _ := rs.Update(tui.ExecDoneMsg{Output: "rolled back 0 statement(s)"})
	finishedRs, ok := finished.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", finished)
	}
	v = stripScreenANSI(finishedRs.View().Content)
	if !strings.Contains(v, "rolled back 0 statement(s)") {
		t.Fatalf("done view wrong:\n%s", v)
	}

	finishedRs2, ok := finished.(*RollbackScreen)
	if !ok {
		t.Fatalf("finished = %T, want *RollbackScreen", finished)
	}
	_, cancel := finishedRs2.Update(databaseKeyPress("esc"))
	if _, ok := cancel().(tui.CanceledMsg); !ok {
		t.Fatalf("esc = %T, want CanceledMsg", cancel())
	}
}

func TestRollbackScreenDryRunFlag(t *testing.T) {
	m := NewRollbackScreen()
	m.adapter = ""
	m.dsn = "data/app.db"
	m.countRaw = "1"
	m.dryRun = true
	m.confirmed = true
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rs, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}
	if !strings.Contains(rs.exec.CLI(), "--dry-run") {
		t.Fatalf("exec CLI = %q, want dry-run", rs.exec.CLI())
	}
	if !strings.Contains(rs.exec.CLI(), "--adapter sqlite") {
		t.Fatalf("exec CLI = %q, want sqlite default", rs.exec.CLI())
	}
}

func TestRollbackScreenExecEscWhileRunning(t *testing.T) {
	m := NewRollbackScreen()
	m.dsn = "data/app.db"
	m.countRaw = "1"
	m.confirmed = true
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rs, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}

	_, cmd := rs.Update(databaseKeyPress("esc"))
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}
}

func TestRollbackScreenFormAbortedCancels(t *testing.T) {
	m := NewRollbackScreen()
	m.form.State = huh.StateAborted

	got, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}
	abortedScreen, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}
	if !abortedScreen.canceled {
		t.Fatal("aborted form must mark canceled")
	}
}

func TestRollbackScreenWindowSizeAndPing(t *testing.T) {
	m := NewRollbackScreen()
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	rs, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}
	if rs.stage != databaseStageForm {
		t.Fatal("resize must stay in form")
	}

	_, cmd := m.Update(databasePing{})
	if cmd != nil {
		t.Fatal("unknown msg in form must be quiet")
	}

	m.stage = databaseStageExec
	_, cmd = m.Update(databasePing{})
	if cmd != nil {
		t.Fatal("unknown msg outside form must be a no-op")
	}
}

func TestRollbackScreenFormKeypressForwards(t *testing.T) {
	m := NewRollbackScreen()
	got, _ := m.Update(databaseKeyPress("x"))
	rs, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}
	if rs.stage != databaseStageForm {
		t.Fatal("typing must stay in form")
	}
}

func TestRollbackScreenExecWithoutArmedExec(t *testing.T) {
	m := NewRollbackScreen()
	got, cmd := m.Update(tui.ExecDoneMsg{Output: "stray"})
	strayRollback, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}
	if strayRollback.stage != databaseStageForm || cmd != nil {
		t.Fatal("stray exec msg with no exec must be a no-op")
	}
}

func TestRollbackScreenExecKeysDelegate(t *testing.T) {
	m := NewRollbackScreen()
	m.dsn = "data/app.db"
	m.countRaw = "1"
	m.confirmed = true
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rs, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}

	kept, cmd := rs.Update(databaseKeyPress("x"))
	keptRollback, ok := kept.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", kept)
	}
	if keptRollback.stage != databaseStageExec {
		t.Fatal("exec must absorb other keys")
	}
	_ = cmd
}

func TestRollbackScreenSQLiteDryRun(t *testing.T) {
	stubDatabaseInteractive(t)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")

	m := NewRollbackScreen()
	m.dsn = dbPath
	m.countRaw = "1"
	m.dryRun = true
	m.confirmed = true
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rs, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}

	msg := rs.exec.Start()
	done, ok := msg.(tui.ExecDoneMsg)
	if !ok {
		t.Fatalf("dry-run = %T (%v), want ExecDoneMsg", msg, msg)
	}
	// Empty tracking table: nothing to plan, nothing printed, no error.
	if strings.Contains(done.Output, dbPath) {
		t.Fatalf("output leaks DSN path:\n%s", done.Output)
	}
}

func TestRollbackScreenSurfacesCoreError(t *testing.T) {
	stubDatabaseInteractive(t)

	// A directory DSN fails inside EnsureMigrationsTable: the error path
	// must stay sanitized and visible.
	m := NewRollbackScreen()
	m.dsn = t.TempDir()
	m.countRaw = "1"
	m.confirmed = true
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rs, ok := got.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", got)
	}

	msg := rs.exec.Start()
	errMsg, ok := msg.(tui.ExecErrMsg)
	if !ok {
		t.Fatalf("bad dsn = %T, want ExecErrMsg", msg)
	}
	if strings.Contains(errMsg.Err.Error(), m.dsn) {
		t.Fatalf("error leaks DSN: %v", errMsg.Err)
	}

	failed, _ := rs.Update(msg)
	failedRs, ok := failed.(*RollbackScreen)
	if !ok {
		t.Fatalf("Update = %T, want *RollbackScreen", failed)
	}
	v := stripScreenANSI(failedRs.View().Content)
	if strings.Contains(v, m.dsn) {
		t.Fatalf("error view leaks DSN:\n%s", v)
	}
}

func TestSeedScreenInitAndGolden(t *testing.T) {
	m := NewSeedScreen()
	if m.Init() == nil {
		t.Fatal("Init must arm the form")
	}

	v := stripScreenANSI(m.View().Content)
	for _, want := range []string{"seed", "zever db seed", "entry", "esc back"} {
		if !strings.Contains(v, want) {
			t.Fatalf("form view missing %q:\n%s", want, v)
		}
	}
	if got := m.CLI(); got != "zever db seed" {
		t.Fatalf("default CLI = %q", got)
	}
}

func TestSeedScreenEscCancels(t *testing.T) {
	m := NewSeedScreen()
	_, cmd := m.Update(databaseKeyPress("esc"))
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}
	if !m.canceled {
		t.Fatal("esc must mark canceled")
	}
}

func TestSeedScreenSubmitRuns(t *testing.T) {
	calls := stubLaunch(t, nil)

	m := NewSeedScreen()
	m.entry = "db/fixtures"
	m.argsRaw = "--only=users --once"
	m.form.State = huh.StateCompleted

	got, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ss, ok := got.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", got)
	}
	if ss.stage != databaseStageExec {
		t.Fatalf("stage = %v, want exec", ss.stage)
	}
	if cmd == nil {
		t.Fatal("submit must start the exec")
	}
	if cli := ss.exec.CLI(); cli != "zever db seed --only=users --once" {
		t.Fatalf("exec CLI = %q", cli)
	}

	msg := ss.exec.Start()
	done, ok := msg.(tui.ExecDoneMsg)
	if !ok {
		t.Fatalf("start = %T (%v), want ExecDoneMsg", msg, msg)
	}
	if !strings.Contains(done.Output, "db/fixtures") {
		t.Fatalf("output = %q, want entry", done.Output)
	}
	assertOneLaunch(t, calls, "db/fixtures", []string{"--only=users", "--once"})

	finished, _ := ss.Update(msg)
	finishedSs, ok := finished.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", finished)
	}
	v := stripScreenANSI(finishedSs.View().Content)
	if !strings.Contains(v, "db/fixtures") || !strings.Contains(v, "zever db seed") {
		t.Fatalf("done view wrong:\n%s", v)
	}
}

func TestSeedScreenDefaultEntry(t *testing.T) {
	t.Chdir(t.TempDir())

	calls := stubLaunch(t, nil)

	m := NewSeedScreen()
	m.entry = "  "
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ss, ok := got.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", got)
	}

	if msg := ss.exec.Start(); msg == nil {
		t.Fatal("start must yield a msg")
	}
	assertOneLaunch(t, calls, defaultSeedEntry, nil)
}

func TestSeedScreenSurfacesLaunchError(t *testing.T) {
	boom := errors.New("child failed")
	_ = stubLaunch(t, boom)

	m := NewSeedScreen()
	m.entry = "db/seed"
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ss, ok := got.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", got)
	}

	msg := ss.exec.Start()
	errMsg, ok := msg.(tui.ExecErrMsg)
	if !ok {
		t.Fatalf("start = %T, want ExecErrMsg", msg)
	}
	if !errors.Is(errMsg.Err, boom) {
		t.Fatalf("err = %v, want boom", errMsg.Err)
	}

	failed, _ := ss.Update(msg)
	failedSs, ok := failed.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", failed)
	}
	v := stripScreenANSI(failedSs.View().Content)
	if !strings.Contains(v, "child failed") {
		t.Fatalf("error view wrong:\n%s", v)
	}
}

func TestSeedScreenBadConfigFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	writeDatabaseFixture(t, dir, "zever.json", "{bad json")
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	// A broken project file must not break the screen: construction falls
	// back to the default seed entry (same as the core withDefaults path).
	m := NewSeedScreen()
	if m.entry != defaultSeedEntry {
		t.Fatalf("entry = %q, want default %q", m.entry, defaultSeedEntry)
	}

	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ss, ok := got.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", got)
	}

	if msg := ss.exec.Start(); msg == nil {
		t.Fatal("start must yield a msg")
	} else if _, ok := msg.(tui.ExecDoneMsg); !ok {
		t.Fatalf("fallback entry must run, got %T", msg)
	}
	assertOneLaunch(t, calls, defaultSeedEntry, nil)
}

func TestSeedScreenFormAbortedCancels(t *testing.T) {
	m := NewSeedScreen()
	m.form.State = huh.StateAborted

	got, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("cmd = %T, want tui.CanceledMsg", cmd())
	}
	abortedSeed, ok := got.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", got)
	}
	if !abortedSeed.canceled {
		t.Fatal("aborted form must mark canceled")
	}
}

func TestSeedScreenFormKeysAndPing(t *testing.T) {
	m := NewSeedScreen()
	got, _ := m.Update(databaseKeyPress("x"))
	ss, ok := got.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", got)
	}
	if ss.stage != databaseStageForm {
		t.Fatal("typing must stay in form")
	}

	_, cmd := m.Update(databasePing{})
	if cmd != nil {
		t.Fatal("unknown msg in form must be quiet")
	}

	m.stage = databaseStageExec
	_, cmd = m.Update(databasePing{})
	if cmd != nil {
		t.Fatal("unknown msg outside form must be a no-op")
	}
}

func TestSeedScreenExecWithoutArmedExec(t *testing.T) {
	m := NewSeedScreen()
	got, cmd := m.Update(tui.ExecErrMsg{Err: errors.New("stray")})
	straySeed, ok := got.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", got)
	}
	if straySeed.stage != databaseStageForm || cmd != nil {
		t.Fatal("stray exec msg with no exec must be a no-op")
	}
}

func TestSeedScreenExecKeysDelegate(t *testing.T) {
	_ = stubLaunch(t, nil)

	m := NewSeedScreen()
	m.entry = "db/seed"
	m.form.State = huh.StateCompleted

	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	ss, ok := got.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", got)
	}

	kept, _ := ss.Update(databaseKeyPress("x"))
	keptSeed, ok := kept.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", kept)
	}
	if keptSeed.stage != databaseStageExec {
		t.Fatal("exec must absorb other keys")
	}

	// Settled exec: esc backs out with cancel.
	finished, _ := ss.Update(tui.ExecDoneMsg{Output: "ok"})
	finishedSs, ok := finished.(*SeedScreen)
	if !ok {
		t.Fatalf("Update = %T, want *SeedScreen", finished)
	}
	_, cmd := finishedSs.Update(databaseKeyPress("esc"))
	if _, ok := cmd().(tui.CanceledMsg); !ok {
		t.Fatalf("esc = %T, want tui.CanceledMsg", cmd())
	}
	if !finishedSs.canceled {
		t.Fatal("esc must mark canceled")
	}
}

// --- teatest: real program wiring ---

func TestDatabaseSeedTeatestEscBack(t *testing.T) {
	m := NewSeedScreen()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	tm.Send(databaseKeyPress("esc"))
	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	final, ok := tm.FinalModel(t).(*SeedScreen)
	if !ok {
		t.Fatalf("final model = %T, want *SeedScreen", tm.FinalModel(t))
	}
	if !final.canceled {
		t.Fatal("esc must mark the seed screen canceled")
	}
}
