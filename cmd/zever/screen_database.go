// Database TUI screens: migrate, rollback, and seed.
//
// Each screen is a huh form (Stack layout, like the runtime screens) that
// builds a core config, then runs it through a kit ExecModel so every stage
// shows the exact equivalent CLI line. Views are pure; all I/O runs inside
// ExecFunc commands, never on the Update path.
//
// DSN secrecy: the DSN field uses huh EchoModePassword (masked bullets on
// screen), every displayed CLI renders the DSN as ***, and every exec output
// and error is scrubbed of the DSN before it reaches a view.
//
// Destructive actions: the migrate drop-columns toggle and the rollback
// confirm both carry explicit unrecoverable-data warnings and default to
// off. Rollback refuses to run until its DANGER confirm is explicitly
// accepted. Migrate always previews first: submit runs a dry-run plan
// (MigrateConfig.DryRun prints the plan without executing or recording
// anything) and only an explicit confirm on that plan starts the apply,
// which re-plans fresh via the same core path as `zever db migrate`.
package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// databaseAdapters are the live adapters the Database screens offer. mysql
// parses as a dialect but has no registered zever db adapter (openZeverDB
// rejects it), so the screens offer exactly sqlite and postgres.
var databaseAdapters = []string{"sqlite", "postgres"}

var (
	// errDatabaseNoFiles is the migrate submit refusal with no schema files.
	errDatabaseNoFiles = errors.New("database: choose at least one schema file")
	// errDatabaseBadCount is the rollback submit refusal for -n < 1.
	errDatabaseBadCount = errors.New("database: count must be a positive integer")
	// errDatabaseDSNRequired is the submit refusal with no DSN.
	errDatabaseDSNRequired = errors.New("database: DSN is required")
	// errDatabaseBadAdapter is the submit refusal for an unregistered adapter.
	errDatabaseBadAdapter = errors.New("database: adapter must be sqlite or postgres")
	// errDatabaseConfirmRequired is the rollback refusal until the DANGER
	// confirm is explicitly accepted.
	errDatabaseConfirmRequired = errors.New("database: confirm the destructive action to proceed")
)

// databaseStage is the lifecycle phase of a database screen. Migrate walks
// form → preview (dry-run exec) → confirm → exec (apply); rollback and seed
// walk form → exec.
type databaseStage int

const (
	// databaseStageForm collects input through the embedded huh form.
	databaseStageForm databaseStage = iota
	// databaseStagePreview runs the dry-run plan inside an ExecModel.
	databaseStagePreview
	// databaseStageConfirm shows the plan and asks for the apply.
	databaseStageConfirm
	// databaseStageExec runs the core and shows its result view.
	databaseStageExec
)

// runDatabaseUpdate advances one form→exec database screen (rollback, seed)
// through the lifecycle both share: WindowSizeMsg goes to the form, esc
// backs out (canceling; in exec phase only once the exec finished), exec
// messages go to the ExecModel once started, and anything else goes to the
// form while in form phase. self is the screen itself, updateForm its form
// handler; stage/exec/canceled mutate in place. Identical to the inlined
// Update bodies it replaces.
func runDatabaseUpdate(msg tea.Msg, self tea.Model, stage *databaseStage, exec *tui.ExecModel, hasExec bool, keys tui.Keymap, canceled *bool, updateForm func(msg tea.Msg) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return updateForm(msg)
	case tea.KeyPressMsg:
		if *stage != databaseStageForm {
			if keys.Back.Matches(msg.String()) && exec.State() != tui.ExecRunning {
				*canceled = true

				return self, func() tea.Msg { return tui.CanceledMsg{} }
			}
			updated, cmd := exec.Update(msg)
			if em, ok := updated.(tui.ExecModel); ok {
				*exec = em
			}

			return self, cmd
		}
		if keys.Back.Matches(msg.String()) {
			*canceled = true

			return self, func() tea.Msg { return tui.CanceledMsg{} }
		}

		return updateForm(msg)
	case tui.ExecDoneMsg, tui.ExecErrMsg:
		if !hasExec {
			return self, nil
		}
		updated, cmd := exec.Update(msg)
		if em, ok := updated.(tui.ExecModel); ok {
			*exec = em
		}

		return self, cmd
	default:
		if *stage == databaseStageForm {
			return updateForm(msg)
		}

		return self, nil
	}
}

// isDatabaseAdapter reports whether name is an offered live adapter.
func isDatabaseAdapter(name string) bool {
	for _, a := range databaseAdapters {
		if strings.TrimSpace(name) == a {
			return true
		}
	}

	return false
}

// databaseDSNPreview renders the DSN for display: empty stays empty (the
// flag is omitted), redacted renders ***, otherwise the trimmed value.
func databaseDSNPreview(dsn string, redact bool) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return ""
	}
	if redact {
		return "***"
	}

	return dsn
}

// buildDatabaseMigrateCLI renders the exact equivalent CLI invocation.
// redact masks the DSN for display; the exec path passes redact=false.
func buildDatabaseMigrateCLI(adapter, dsn string, files []string, dryRun, dropColumns, redact bool) string {
	var b strings.Builder

	b.WriteString("zever db migrate")
	if a := strings.TrimSpace(adapter); a != "" {
		b.WriteString(" --adapter " + cliQuote(a))
	}
	if d := databaseDSNPreview(dsn, redact); d != "" {
		b.WriteString(" --dsn " + cliQuote(d))
	}
	for _, f := range files {
		b.WriteString(" " + cliQuote(f))
	}
	if dryRun {
		b.WriteString(" --dry-run")
	}
	if dropColumns {
		b.WriteString(" --drop-columns")
	}

	return b.String()
}

// migrateDatabaseArgs builds the argv for runDBMigrate from a resolved config.
func migrateDatabaseArgs(adapter, dsn string, files []string, dryRun, dropColumns bool) []string {
	args := []string{"--adapter", strings.TrimSpace(adapter)}
	if d := strings.TrimSpace(dsn); d != "" {
		args = append(args, "--dsn", d)
	}
	args = append(args, files...)
	if dryRun {
		args = append(args, "--dry-run")
	}
	if dropColumns {
		args = append(args, "--drop-columns")
	}

	return args
}

// buildDatabaseRollbackCLI renders the exact equivalent CLI invocation.
func buildDatabaseRollbackCLI(adapter, dsn string, count int, dryRun, redact bool) string {
	var b strings.Builder

	b.WriteString("zever db rollback")
	if a := strings.TrimSpace(adapter); a != "" {
		b.WriteString(" --adapter " + cliQuote(a))
	}
	if d := databaseDSNPreview(dsn, redact); d != "" {
		b.WriteString(" --dsn " + cliQuote(d))
	}
	b.WriteString(" -n " + strconv.Itoa(count))
	if dryRun {
		b.WriteString(" --dry-run")
	}

	return b.String()
}

// rollbackDatabaseArgs builds the argv for runDBRollback from a resolved config.
func rollbackDatabaseArgs(adapter, dsn string, count int, dryRun bool) []string {
	args := []string{"--adapter", strings.TrimSpace(adapter), "--dsn", strings.TrimSpace(dsn), "-n", strconv.Itoa(count)}
	if dryRun {
		args = append(args, "--dry-run")
	}

	return args
}

// buildDatabaseSeedCLI renders the exact equivalent CLI invocation. The
// entrypoint is config-side (like the runtime screens), so only the
// passthrough args render.
func buildDatabaseSeedCLI(args []string) string {
	if len(args) == 0 {
		return "zever db seed"
	}

	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, cliQuote(a))
	}

	return "zever db seed " + strings.Join(quoted, " ")
}

// splitDatabaseFileList splits the free-text files field on commas,
// semicolons, and whitespace into separate paths.
func splitDatabaseFileList(s string) []string {
	s = strings.ReplaceAll(s, ",", " ")
	s = strings.ReplaceAll(s, ";", " ")

	return strings.Fields(s)
}

// splitExtraArgs splits the free-text args field. Fields stay separate argv
// elements (no shell), matching the runtime screens.
func splitDatabaseArgs(s string) []string { return strings.Fields(s) }

// sanitizeDatabaseOutput scrubs every secret from text before it reaches a
// view or log. Empty secrets are skipped.
func sanitizeDatabaseOutput(text string, secrets ...string) string {
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, "***")
	}

	return text
}

// databasePipe opens the OS pipe carrying captured core output. Seam so the
// pipe-failure branch is testable.
var databasePipe = os.Pipe

// captureDatabaseOutput runs fn with os.Stdout piped and returns what it
// printed plus fn's error. The migrate/rollback cores print their plans and
// results instead of returning them, so the screens capture that stream
// into the ExecModel result text.
func captureDatabaseOutput(fn func() error) (string, error) {
	orig := os.Stdout
	r, w, err := databasePipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	fnErr := fn()
	_ = w.Close()
	os.Stdout = orig
	out, _ := io.ReadAll(r)
	_ = r.Close()

	return string(out), fnErr
}

// runDatabaseMigrateCapture runs runDBMigrate with a resolved config and
// returns its printed output, scrubbed of the DSN. The error text is
// scrubbed too: sqlite open failures can echo the file path.
func runDatabaseMigrateCapture(cfg MigrateConfig) tui.ExecFunc {
	args := migrateDatabaseArgs(cfg.Adapter, cfg.DSN, cfg.Files, cfg.DryRun, cfg.DropColumns)

	return func(context.Context) (string, error) {
		out, err := captureDatabaseOutput(func() error { return runDBMigrate(args) })
		out = sanitizeDatabaseOutput(out, cfg.DSN)
		if err != nil {
			return out, errors.New(sanitizeDatabaseOutput(err.Error(), cfg.DSN))
		}

		return out, nil
	}
}

// runDatabaseRollbackCapture runs runDBRollback with a resolved config and
// returns its printed output, scrubbed of the DSN.
func runDatabaseRollbackCapture(cfg RollbackConfig) tui.ExecFunc {
	args := rollbackDatabaseArgs(cfg.Adapter, cfg.DSN, cfg.Count, cfg.DryRun)

	return func(context.Context) (string, error) {
		out, err := captureDatabaseOutput(func() error { return runDBRollback(args) })
		out = sanitizeDatabaseOutput(out, cfg.DSN)
		if err != nil {
			return out, errors.New(sanitizeDatabaseOutput(err.Error(), cfg.DSN))
		}

		return out, nil
	}
}

// runDatabaseSeedCapture runs the seed entrypoint through the same execLaunch
// seam runDBSeed uses, so stubLaunch covers it in tests.
func runDatabaseSeedCapture(entry string, args []string) tui.ExecFunc {
	return func(context.Context) (string, error) {
		if err := execLaunch(entry, args); err != nil {
			return "", err
		}

		return "seed entrypoint " + entry + " finished", nil
	}
}

// RegisterDatabaseScreens exposes this group's dashboard entries: group,
// name, description, exact equivalent CLI, and the owning screen
// constructor name. No models are built here.
func RegisterDatabaseScreens() []tui.Entry {
	return []tui.Entry{
		{Group: tui.GroupDatabase, Name: "migrate", Desc: "run pending migrations", CLI: "zever db migrate", Screen: "NewMigrateScreen"},
		{Group: tui.GroupDatabase, Name: "rollback", Desc: "undo recent migration statements", CLI: "zever db rollback", Screen: "NewRollbackScreen"},
		{Group: tui.GroupDatabase, Name: "seed", Desc: "seed development data", CLI: "zever db seed", Screen: "NewSeedScreen"},
	}
}

// --- migrate screen ---

// MigrateScreen collects adapter/DSN/files plus the dry-run and
// drop-columns toggles, previews the dry-run plan, and applies it only on
// explicit confirm. The DSN field is password-masked and never rendered.
type MigrateScreen struct {
	stage       databaseStage
	form        *huh.Form
	adapter     string
	dsn         string
	filesRaw    string
	dryRun      bool
	dropColumns bool
	err         error
	canceled    bool
	exec        tui.ExecModel
	hasExec     bool
	theme       tui.Theme
	keys        tui.Keymap
}

// NewMigrateScreen returns a migrate screen with sqlite preselected. The huh
// inputs bind directly to the struct fields, laid out with the Stack layout.
func NewMigrateScreen() *MigrateScreen {
	m := &MigrateScreen{adapter: "sqlite", theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Adapter").
				Description("database backend (sqlite and postgres only)").
				Options(huh.NewOption("sqlite", "sqlite"), huh.NewOption("postgres", "postgres")).
				Value(&m.adapter),
			huh.NewInput().
				Title("DSN").
				Description("sqlite file path or postgres connection string — masked, never shown back").
				Placeholder("data/app.db").
				EchoMode(huh.EchoModePassword).
				Value(&m.dsn),
			huh.NewInput().
				Title("Schema files").
				Description("space-separated .zen schema paths").
				Placeholder("schema/app.zen").
				Value(&m.filesRaw),
			huh.NewConfirm().
				Title("Preview only (dry run)?").
				Description("print the plan without executing or recording anything").
				Value(&m.dryRun),
			huh.NewConfirm().
				Title("Drop undeclared columns?").
				Description("WARNING: emits DROP COLUMN for live columns missing from the schema — dropped data is unrecoverable").
				Value(&m.dropColumns),
		),
	).WithLayout(huh.LayoutStack).WithWidth(60)

	return m
}

// CLI returns the live redacted equivalent CLI string from current values.
func (m *MigrateScreen) CLI() string {
	return buildDatabaseMigrateCLI(m.adapter, m.dsn, splitDatabaseFileList(m.filesRaw), m.dryRun, m.dropColumns, true)
}

// confirmCLI returns the redacted CLI for the confirm stage: the apply
// invocation when a run is offered, the dry-run invocation otherwise.
func (m *MigrateScreen) confirmCLI() string {
	return buildDatabaseMigrateCLI(m.adapter, m.dsn, splitDatabaseFileList(m.filesRaw), m.dryRun, m.dropColumns, true)
}

// Init implements tea.Model. It arms the embedded form; work starts on
// preview confirm, never here, so construction does no I/O.
func (m *MigrateScreen) Init() tea.Cmd { return m.form.Init() }

// failForm records a submit refusal and reopens the form for correction.
func (m *MigrateScreen) failForm(cmd tea.Cmd, err error) (tea.Model, tea.Cmd) {
	m.err = err
	m.form.State = huh.StateNormal

	return m, cmd
}

// submitPreview validates the form and starts the dry-run preview exec.
func (m *MigrateScreen) submitPreview(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	adapter := strings.TrimSpace(m.adapter)
	if adapter == "" {
		adapter = "sqlite"
	}
	if !isDatabaseAdapter(adapter) {
		return m.failForm(cmd, errDatabaseBadAdapter)
	}
	files := splitDatabaseFileList(m.filesRaw)
	if len(files) == 0 {
		return m.failForm(cmd, errDatabaseNoFiles)
	}
	m.adapter = adapter
	m.err = nil
	dsn := strings.TrimSpace(m.dsn)
	cfg := MigrateConfig{Adapter: adapter, DSN: dsn, Files: files, DryRun: true, DropColumns: m.dropColumns}
	m.exec = tui.NewExec("migrate (dry run)", buildDatabaseMigrateCLI(adapter, dsn, files, true, m.dropColumns, true), runDatabaseMigrateCapture(cfg))
	m.hasExec = true
	m.stage = databaseStagePreview

	return m, tea.Batch(m.exec.Init(), m.exec.Start)
}

// updateForm forwards msg to the huh form and starts the preview on
// completion. Aborting the form cancels the screen.
func (m *MigrateScreen) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.form.Update(msg)
	switch m.form.State { //nolint:exhaustive // default returns unchanged while the form is still in progress
	case huh.StateCompleted:
		return m.submitPreview(cmd)
	case huh.StateAborted:
		m.canceled = true

		return m, func() tea.Msg { return tui.CanceledMsg{} }
	default:
		return m, cmd
	}
}

// absorbExec forwards msg to the armed exec and reports whether the run
// reached a terminal state.
func (m *MigrateScreen) absorbExec(msg tea.Msg) (tea.Cmd, bool) {
	updated, cmd := m.exec.Update(msg)
	if em, ok := updated.(tui.ExecModel); ok {
		m.exec = em
	}
	st := m.exec.State()

	return cmd, st == tui.ExecDone || st == tui.ExecFailed
}

// confirmSelect handles enter on the plan confirm: dry-run-only goes back
// to the form, otherwise the apply exec starts.
func (m *MigrateScreen) confirmSelect() (tea.Model, tea.Cmd) {
	if m.dryRun {
		m.stage = databaseStageForm

		return m, nil
	}
	cfg := MigrateConfig{
		Adapter:     m.adapter,
		DSN:         strings.TrimSpace(m.dsn),
		Files:       splitDatabaseFileList(m.filesRaw),
		DropColumns: m.dropColumns,
	}
	m.exec = tui.NewExec("migrate", buildDatabaseMigrateCLI(cfg.Adapter, cfg.DSN, cfg.Files, false, cfg.DropColumns, true), runDatabaseMigrateCapture(cfg))
	m.stage = databaseStageExec

	return m, tea.Batch(m.exec.Init(), m.exec.Start)
}

// Update implements tea.Model. Esc backs out (canceling the screen); the
// preview exec cancels in flight through the kit model. No I/O: work starts
// as returned Cmds.
func (m *MigrateScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.updateForm(msg)
	case tea.KeyPressMsg:
		key := msg.String()
		switch m.stage {
		case databaseStageForm:
			if m.keys.Back.Matches(key) {
				m.canceled = true

				return m, func() tea.Msg { return tui.CanceledMsg{} }
			}

			return m.updateForm(msg)
		case databaseStagePreview:
			cmd, done := m.absorbExec(msg)
			if done {
				m.stage = databaseStageConfirm

				return m, nil
			}

			return m, cmd
		case databaseStageConfirm:
			switch {
			case m.keys.Back.Matches(key):
				m.canceled = true

				return m, func() tea.Msg { return tui.CanceledMsg{} }
			case m.keys.Select.Matches(key):
				return m.confirmSelect()
			default:
				return m, nil
			}
		case databaseStageExec:
			if m.keys.Back.Matches(key) && m.exec.State() != tui.ExecRunning {
				m.canceled = true

				return m, func() tea.Msg { return tui.CanceledMsg{} }
			}
			updated, cmd := m.exec.Update(msg)
			if em, ok := updated.(tui.ExecModel); ok {
				m.exec = em
			}

			return m, cmd
		default:
			return m, nil
		}
	case tui.ExecDoneMsg, tui.ExecErrMsg:
		if !m.hasExec {
			return m, nil
		}
		_, done := m.absorbExec(msg)
		if done && m.stage == databaseStagePreview {
			m.stage = databaseStageConfirm

			return m, nil
		}

		return m, nil
	default:
		if m.stage == databaseStageForm {
			return m.updateForm(msg)
		}

		return m, nil
	}
}

// View implements tea.Model. Pure: the DSN never renders (masked input,
// redacted CLI, scrubbed output).
func (m *MigrateScreen) View() tea.View {
	var b strings.Builder

	b.WriteString(m.theme.Title.Render("migrate"))
	b.WriteString("\n\n")
	switch m.stage { //nolint:exhaustive // default renders the form view, covering databaseStageForm and future stages
	case databaseStagePreview, databaseStageExec:
		b.WriteString(stripScreenANSI(m.exec.View().Content))
	case databaseStageConfirm:
		if out := sanitizeDatabaseOutput(m.exec.Output(), m.dsn); out != "" {
			b.WriteString(out)
			b.WriteString("\n\n")
		}
		if m.exec.State() == tui.ExecFailed && m.exec.Err() != nil {
			b.WriteString(m.theme.Fail.Render("error: " + sanitizeDatabaseOutput(m.exec.Err().Error(), m.dsn)))
			b.WriteString("\n\n")
		}
		if m.dryRun {
			b.WriteString(m.theme.Hint.Render("dry run — no changes applied"))
		} else {
			b.WriteString(m.theme.Fail.Render("Apply these statements?"))
		}
		b.WriteString("\n")
		b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.confirmCLI()))
		b.WriteString("\n")
		if m.dryRun {
			b.WriteString(m.theme.Hint.Render("enter back · esc back"))
		} else {
			b.WriteString(m.theme.Hint.Render("enter apply · esc back"))
		}
	default:
		b.WriteString(stripScreenANSI(m.form.View()))
		b.WriteString("\n\n")
		if m.err != nil {
			b.WriteString(m.theme.Fail.Render("error: " + m.err.Error()))
			b.WriteString("\n\n")
		}
		b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
		b.WriteString("\n")
		b.WriteString(m.theme.Hint.Render("enter next · esc back"))
	}

	return tea.NewView(b.String())
}

// --- rollback screen ---

// RollbackScreen collects adapter/DSN/count plus the DANGER confirm and
// runs the rollback. Submit refuses until the confirm is explicitly
// accepted; the warning banners the form so the cost is visible before
// anything runs.
type RollbackScreen struct {
	stage     databaseStage
	form      *huh.Form
	adapter   string
	dsn       string
	countRaw  string
	dryRun    bool
	confirmed bool
	err       error
	canceled  bool
	exec      tui.ExecModel
	hasExec   bool
	theme     tui.Theme
	keys      tui.Keymap
}

// NewRollbackScreen returns a rollback screen with sqlite preselected,
// count 1, and the DANGER confirm defaulted to off.
func NewRollbackScreen() *RollbackScreen {
	m := &RollbackScreen{adapter: "sqlite", countRaw: "1", theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Adapter").
				Description("database backend (sqlite and postgres only)").
				Options(huh.NewOption("sqlite", "sqlite"), huh.NewOption("postgres", "postgres")).
				Value(&m.adapter),
			huh.NewInput().
				Title("DSN").
				Description("sqlite file path or postgres connection string — masked, never shown back").
				Placeholder("data/app.db").
				EchoMode(huh.EchoModePassword).
				Value(&m.dsn),
			huh.NewInput().
				Title("Count (-n)").
				Description("schema_migrations ROWS to undo, newest first").
				Placeholder("1").
				Value(&m.countRaw),
			huh.NewConfirm().
				Title("Preview only (dry run)?").
				Description("print the inverse statements without executing them").
				Value(&m.dryRun),
			huh.NewConfirm().
				Title("Roll back and destroy?").
				Description("DANGER: undoing ADD COLUMN drops the column and every value written to it — unrecoverable. Undoing DROP COLUMN recreates the column EMPTY; the original data is gone. Use dry run first.").
				Affirmative("Yes, roll back").
				Negative("No, cancel").
				Value(&m.confirmed),
		),
	).WithLayout(huh.LayoutStack).WithWidth(60)

	return m
}

// rollbackCount parses the -n field: a positive integer row count.
func (m *RollbackScreen) rollbackCount() (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(m.countRaw))
	if err != nil || n < 1 {
		return 0, errDatabaseBadCount
	}

	return n, nil
}

// CLI returns the live redacted equivalent CLI string from current values.
func (m *RollbackScreen) CLI() string {
	n, _ := m.rollbackCount()

	return buildDatabaseRollbackCLI(m.adapter, m.dsn, n, m.dryRun, true)
}

// Init implements tea.Model. It arms the embedded form; work starts on
// submit, never here, so construction does no I/O.
func (m *RollbackScreen) Init() tea.Cmd { return m.form.Init() }

// failForm records a submit refusal and reopens the form for correction.
func (m *RollbackScreen) failForm(cmd tea.Cmd, err error) (tea.Model, tea.Cmd) {
	m.err = err
	m.form.State = huh.StateNormal

	return m, cmd
}

// submitExec validates the form — including the explicit DANGER confirm —
// and starts the rollback exec.
func (m *RollbackScreen) submitExec(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	adapter := strings.TrimSpace(m.adapter)
	if adapter == "" {
		adapter = "sqlite"
	}
	if !isDatabaseAdapter(adapter) {
		return m.failForm(cmd, errDatabaseBadAdapter)
	}
	dsn := strings.TrimSpace(m.dsn)
	if dsn == "" {
		return m.failForm(cmd, errDatabaseDSNRequired)
	}
	n, err := m.rollbackCount()
	if err != nil {
		return m.failForm(cmd, err)
	}
	if !m.confirmed {
		return m.failForm(cmd, errDatabaseConfirmRequired)
	}
	m.adapter = adapter
	m.err = nil
	cfg := RollbackConfig{Adapter: adapter, DSN: dsn, Count: n, DryRun: m.dryRun}
	m.exec = tui.NewExec("rollback", buildDatabaseRollbackCLI(adapter, dsn, n, m.dryRun, true), runDatabaseRollbackCapture(cfg))
	m.hasExec = true
	m.stage = databaseStageExec

	return m, tea.Batch(m.exec.Init(), m.exec.Start)
}

// updateForm forwards msg to the huh form and submits on completion.
// Aborting the form cancels the screen.
func (m *RollbackScreen) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.form.Update(msg)
	switch m.form.State { //nolint:exhaustive // default returns unchanged while the form is still in progress
	case huh.StateCompleted:
		return m.submitExec(cmd)
	case huh.StateAborted:
		m.canceled = true

		return m, func() tea.Msg { return tui.CanceledMsg{} }
	default:
		return m, cmd
	}
}

// Update implements tea.Model. Esc backs out (canceling the screen); the
// exec cancels in flight through the kit model. No I/O: work starts as
// returned Cmds.
func (m *RollbackScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return runDatabaseUpdate(msg, m, &m.stage, &m.exec, m.hasExec, m.keys, &m.canceled, m.updateForm)
}

// View implements tea.Model. Pure: the DSN never renders, and the DANGER
// warning banners the form.
func (m *RollbackScreen) View() tea.View {
	var b strings.Builder

	b.WriteString(m.theme.Title.Render("rollback"))
	b.WriteString("\n\n")
	if m.stage == databaseStageExec {
		b.WriteString(stripScreenANSI(m.exec.View().Content))

		return tea.NewView(b.String())
	}
	b.WriteString(m.theme.Fail.Render("DANGER: rollback can destroy data"))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("dropped columns and their values are unrecoverable — dry-run first"))
	b.WriteString("\n\n")
	b.WriteString(stripScreenANSI(m.form.View()))
	b.WriteString("\n\n")
	if m.err != nil {
		b.WriteString(m.theme.Fail.Render("error: " + m.err.Error()))
		b.WriteString("\n\n")
	}
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter next · esc back"))

	return tea.NewView(b.String())
}

// --- seed screen ---

// SeedScreen collects the seed entrypoint and its passthrough args, then
// runs the entrypoint through the launcher seam.
type SeedScreen struct {
	stage    databaseStage
	form     *huh.Form
	entry    string
	argsRaw  string
	canceled bool
	exec     tui.ExecModel
	hasExec  bool
	theme    tui.Theme
	keys     tui.Keymap
}

// NewSeedScreen returns a seed screen over the project seed entry. The huh
// inputs bind directly to the struct fields, laid out with the Stack layout.
func NewSeedScreen() *SeedScreen {
	project, _ := loadProjectConfig()
	m := &SeedScreen{entry: project.withDefaults().SeedEntry, theme: tui.NewTheme(), keys: tui.DefaultKeymap()}
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Seed entry").
				Description("seed entrypoint package directory").
				Placeholder(defaultSeedEntry).
				Value(&m.entry),
			huh.NewInput().
				Title("Args").
				Description("space-separated, passed through untouched to the entrypoint").
				Placeholder("--only=users").
				Value(&m.argsRaw),
		),
	).WithLayout(huh.LayoutStack).WithWidth(60)

	return m
}

// resolvedEntry returns the form entry, falling back to the project default.
func (m *SeedScreen) resolvedEntry() string {
	if e := strings.TrimSpace(m.entry); e != "" {
		return e
	}

	return defaultSeedEntry
}

// CLI returns the live equivalent CLI string from the current args.
func (m *SeedScreen) CLI() string { return buildDatabaseSeedCLI(splitDatabaseArgs(m.argsRaw)) }

// Init implements tea.Model. It arms the embedded form; work starts on
// submit, never here, so construction does no I/O.
func (m *SeedScreen) Init() tea.Cmd { return m.form.Init() }

// updateForm forwards msg to the huh form and runs the entrypoint on
// completion. Aborting the form cancels the screen.
func (m *SeedScreen) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.form.Update(msg)
	switch m.form.State { //nolint:exhaustive // default returns unchanged while the form is still in progress
	case huh.StateCompleted:
		entry := m.resolvedEntry()
		args := splitDatabaseArgs(m.argsRaw)
		if len(args) == 0 {
			args = nil
		}
		m.exec = tui.NewExec("seed", buildDatabaseSeedCLI(args), runDatabaseSeedCapture(entry, args))
		m.hasExec = true
		m.stage = databaseStageExec

		return m, tea.Batch(m.exec.Init(), m.exec.Start)
	case huh.StateAborted:
		m.canceled = true

		return m, func() tea.Msg { return tui.CanceledMsg{} }
	default:
		return m, cmd
	}
}

// Update implements tea.Model. Esc backs out (canceling the screen); the
// exec cancels in flight through the kit model. No I/O: work starts as
// returned Cmds.
func (m *SeedScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return runDatabaseUpdate(msg, m, &m.stage, &m.exec, m.hasExec, m.keys, &m.canceled, m.updateForm)
}

// View implements tea.Model. Pure.
func (m *SeedScreen) View() tea.View {
	var b strings.Builder

	b.WriteString(m.theme.Title.Render("seed"))
	b.WriteString("\n\n")
	if m.stage == databaseStageExec {
		b.WriteString(stripScreenANSI(m.exec.View().Content))

		return tea.NewView(b.String())
	}
	b.WriteString(stripScreenANSI(m.form.View()))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("equivalent CLI: " + m.CLI()))
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("enter next · esc back"))

	return tea.NewView(b.String())
}
