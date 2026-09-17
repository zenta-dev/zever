// Runtime TUI screens: serve, dev, queue:work/schedule:run, and tinker.
//
// Each screen is a small huh form (entry path, extra args) that confirms into
// a kit log pane with live child output. Esc in the form cancels
// (tui.CanceledMsg); esc in the pane stops the child and asks the parent to
// pop (tui.LogBackMsg). Views are pure; all I/O runs in tea.Cmds.
//
// Child-lifecycle contract (no orphans): every screen owns its child handle.
// The serve/worker launchers spawn `go run` in its own process group
// (isolateProcessGroup) and stop it with SIGTERM-to-group, SIGKILL-to-group
// past the grace period, then Wait (reap). The dev screen runs devLoop under
// a cancellable context: cancel unwinds the loop, whose deferred child.stop
// reaps the server. The tinker screen owns its shim client: esc closes it,
// which cancels the shim context and Waits the subprocess.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/traefik/yaegi/interp"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// RegisterRuntimeScreens returns the Runtime group dashboard entries. The
// schedule:run entry shares the queue:work screen: the scheduler runs inside
// the worker process, so there is exactly one screen for both commands.
func RegisterRuntimeScreens() []tui.Entry {
	return []tui.Entry{
		{Group: tui.GroupRuntime, Name: "serve", Desc: "run the HTTP server", CLI: "zever serve", Screen: "NewServeScreen"},
		{Group: tui.GroupRuntime, Name: "dev", Desc: "run with live reload", CLI: "zever dev", Screen: "NewDevScreen"},
		{Group: tui.GroupRuntime, Name: "queue:work", Desc: "run the background worker (alias: zever schedule:run)", CLI: "zever queue:work", Screen: "NewQueueWorkScreen"},
		{Group: tui.GroupRuntime, Name: "schedule:run", Desc: "alias for queue:work: the worker runs the scheduler in-process", CLI: "zever schedule:run", Screen: "NewQueueWorkScreen"},
		{Group: tui.GroupRuntime, Name: "tinker", Desc: "open an interactive REPL", CLI: "zever tinker", Screen: "NewTinkerScreen"},
	}
}

// runtimeStage is the lifecycle phase of a runtime screen.
type runtimeStage int

const (
	// runtimeForm collects entry/args; runtimeRunning streams child output;
	// runtimeDone shows the exit note.
	runtimeForm runtimeStage = iota
	runtimeRunning
	runtimeDone
)

// runtimeLoadProject resolves the project layout. Seam so screens can be
// built over a stubbed config in tests.
var runtimeLoadProject = loadProjectConfig

// splitExtraArgs splits the free-text args field. Fields stay separate
// argv elements (no shell), matching buildGoRunArgs.
func splitExtraArgs(s string) []string { return strings.Fields(s) }

// --- child process ownership (no orphans) ---

// runtimeChild is a running `go run` child plus the goroutine reaping it.
// stop is idempotent: a reaped child returns immediately.
type runtimeChild struct {
	cmd  *exec.Cmd
	done chan struct{}
}

// newRuntimeChild takes ownership of a started command and reaps it in the
// background. Callers must eventually call stop.
func newRuntimeChild(cmd *exec.Cmd) *runtimeChild {
	c := &runtimeChild{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(c.done)
	}()
	return c
}

// alive reports whether the child has not been reaped yet.
func (c *runtimeChild) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// stop asks the whole process group to shut down gracefully (the `go run`
// wrapper ignores direct signals, so group delivery is required), hard-kills
// past grace, and returns only once the process is reaped.
func (c *runtimeChild) stop(grace time.Duration) {
	if !c.alive() {
		return
	}
	forwardSignal(c.cmd, syscall.SIGTERM)
	select {
	case <-c.done:
	case <-time.After(grace):
		killProcessGroup(c.cmd)
		<-c.done
	}
}

// runtimePipe opens the OS pipe carrying child output. Seam so the
// pipe-failure branch is testable.
var runtimePipe = os.Pipe

// runtimeStartPiped starts `go run <entry> <args...>` in its own process
// group with stdout/stderr captured to the returned reader. Stdin is left
// nil (null device) so the child can never steal the TUI's terminal. Seam
// so tests run stub commands instead of the Go toolchain.
var runtimeStartPiped = defaultStartRuntimePiped

// defaultStartRuntimePiped is the production runtimeStartPiped.
func defaultStartRuntimePiped(entry string, args []string) (*runtimeChild, *bufio.Reader, error) {
	//nolint:gosec // entry comes from the developer's own project config
	cmd := exec.CommandContext(context.Background(), "go", buildGoRunArgs(entry, args)...)
	isolateProcessGroup(cmd)
	r, w, err := runtimePipe()
	if err != nil {
		return nil, nil, fmt.Errorf("zever: runtime pipe: %w", err)
	}
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Start(); err != nil {
		_ = r.Close()
		_ = w.Close()
		return nil, nil, fmt.Errorf("zever: launch %q: %w", entry, err)
	}
	_ = w.Close() // parent keeps the read end; the child holds its own copy
	return newRuntimeChild(cmd), bufio.NewReader(r), nil
}

// --- line streaming (viewport pattern) ---
//
// Long-running output never uses tea.Exec/ExecProcess: those pause the
// event loop and hide the UI until the child exits. Instead the child writes
// to a pipe and one tea.Cmd per line carries text back as Msgs; the parent
// Update Appends into the kit log pane (follow-tail) and re-issues the read.
// No I/O happens on the Update path.

// runtimeLineMsg carries one child output line to the log pane.
type runtimeLineMsg struct{ line string }

// runtimeDoneMsg reports the output stream reached EOF.
type runtimeDoneMsg struct{}

// runtimeReadCmd reads a single line off r. Re-issue from Update until
// runtimeDoneMsg arrives.
func runtimeReadCmd(r *bufio.Reader) tea.Cmd {
	return func() tea.Msg {
		line, err := r.ReadString('\n')
		if err != nil {
			if trimmed := strings.TrimRight(line, "\r\n"); trimmed != "" {
				return runtimeLineMsg{line: trimmed}
			}
			return runtimeDoneMsg{}
		}
		return runtimeLineMsg{line: strings.TrimRight(line, "\r\n")}
	}
}

// --- serve / queue:work launcher screens ---

// runtimeLauncher is the shared form→log-pane machine for the `go run`
// launcher commands (serve, queue:work). ServeScreen and QueueWorkScreen
// embed it with different titles, CLI bases, and entries.
type runtimeLauncher struct {
	title     string
	cliBase   string
	aliasNote string
	stage     runtimeStage
	form      *huh.Form
	project   ProjectConfig
	entry     string
	extra     string
	cli       string
	log       tui.LogModel
	child     *runtimeChild
	reader    *bufio.Reader
	canceled  bool
	theme     tui.Theme
	keys      tui.Keymap
	width     int
	height    int
}

// newRuntimeLauncher builds the form over the project defaults. The huh
// inputs bind directly to the struct fields, laid out with the Stack layout.
func newRuntimeLauncher(title, cliBase, aliasNote, entryTitle, entryDesc, entryDefault string, project ProjectConfig) runtimeLauncher {
	s := runtimeLauncher{
		title: title, cliBase: cliBase, aliasNote: aliasNote,
		project: project, entry: entryDefault,
		theme: tui.NewTheme(), keys: tui.DefaultKeymap(), log: tui.NewLog(80, 18),
	}
	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title(entryTitle).Description(entryDesc).Placeholder(entryDefault).Value(&s.entry),
			huh.NewInput().Title("Extra args").Description("passed through untouched").Placeholder("-addr :9090").Value(&s.extra),
		),
	).WithLayout(huh.LayoutStack).WithWidth(60)
	return s
}

// resolvedEntry returns the form entry, falling back to the project default.
// The value passes through to the launcher unchanged (no widening).
func (s *runtimeLauncher) resolvedEntry() string {
	if e := strings.TrimSpace(s.entry); e != "" {
		return e
	}
	return s.project.ServerEntry
}

// entryValue reports the effective entry (test seam).
func (s *runtimeLauncher) entryValue() string { return s.resolvedEntry() }

// buildCLI renders the exact equivalent CLI string. Entry is config-side
// (not a flag), so only the passthrough args render.
func (s *runtimeLauncher) buildCLI(entry string, args []string) string {
	_ = entry // config-side: selects the package, never rendered as a flag
	if len(args) == 0 {
		return s.cliBase
	}
	return s.cliBase + " " + strings.Join(args, " ")
}

// CLI returns the live equivalent CLI string from the current form values.
func (s *runtimeLauncher) CLI() string {
	return s.buildCLI(s.resolvedEntry(), splitExtraArgs(s.extra))
}

// attachChild wires a running child and its output stream (test seam).
func (s *runtimeLauncher) attachChild(c *runtimeChild, r *bufio.Reader) {
	s.child = c
	s.reader = r
}

// Stop terminates the child (group SIGTERM, group SIGKILL past grace) and
// releases the handle. Safe to call with no child.
func (s *runtimeLauncher) Stop() {
	if s.child == nil {
		return
	}
	s.child.stop(devStopGrace)
	s.child = nil
}

// Init implements tea.Model.
func (s *runtimeLauncher) Init() tea.Cmd { return s.form.Init() }

// startRunning launches the child and enters the log pane.
func (s *runtimeLauncher) startRunning() tea.Cmd {
	args := splitExtraArgs(s.extra)
	entry := s.resolvedEntry()
	s.cli = s.buildCLI(entry, args)
	child, r, err := runtimeStartPiped(entry, args)
	if err != nil {
		s.stage = runtimeDone
		s.log.Append("$ "+s.cli, "error: "+err.Error())
		return nil
	}
	s.attachChild(child, r)
	s.stage = runtimeRunning
	s.log.Append("$ " + s.cli)
	return runtimeReadCmd(s.reader)
}

// updateForm forwards msg to the huh form and advances on completion. It
// mutates the launcher in place and returns only the command: every path
// returns s itself, so callers never needed the model.
func (s *runtimeLauncher) updateForm(msg tea.Msg) tea.Cmd {
	_, cmd := s.form.Update(msg)
	switch s.form.State {
	case huh.StateCompleted:
		return s.startRunning()
	case huh.StateAborted:
		s.canceled = true
		return func() tea.Msg { return tui.CanceledMsg{} }
	default:
		return cmd
	}
}

// update implements the shared dispatch. Pure except for child shutdown,
// which runs in the returned tea.Cmd, never on the Update path. It mutates
// the launcher in place and returns only the command: every updateForm path
// returns s itself, so the model out-value was never used.
func (s *runtimeLauncher) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			s.width = msg.Width
			s.form = s.form.WithWidth(max(s.width-4, 20))
		}
		if msg.Height > 0 {
			s.height = msg.Height
		}
		lm, cmd := s.log.Update(msg)
		if l, ok := lm.(*tui.LogModel); ok {
			s.log = *l
		}
		return cmd
	case tea.KeyPressMsg:
		if s.keys.Back.Matches(msg.String()) {
			if s.stage == runtimeForm {
				s.canceled = true
				return func() tea.Msg { return tui.CanceledMsg{} }
			}
			return func() tea.Msg { s.Stop(); return tui.LogBackMsg{} }
		}
		if s.stage == runtimeForm {
			return s.updateForm(msg)
		}
		lm, cmd := s.log.Update(msg)
		if l, ok := lm.(*tui.LogModel); ok {
			s.log = *l
		}
		return cmd
	case runtimeLineMsg:
		if s.stage != runtimeRunning {
			return nil
		}
		s.log.Append(msg.line)
		return runtimeReadCmd(s.reader)
	case runtimeDoneMsg:
		if s.stage == runtimeRunning {
			s.stage = runtimeDone
			s.child = nil
			s.log.Append("exit: child process ended (esc back)")
		}
		return nil
	default:
		if s.stage == runtimeForm {
			return s.updateForm(msg)
		}
		return nil
	}
}

// view renders title, form or log pane, and the exact CLI string.
func (s *runtimeLauncher) view() tea.View {
	var b strings.Builder
	b.WriteString(s.theme.Title.Render(s.title))
	b.WriteString("\n\n")
	if s.stage == runtimeForm {
		b.WriteString(s.form.View())
		b.WriteString("\n\n")
		if s.aliasNote != "" {
			b.WriteString(s.theme.Hint.Render(s.aliasNote))
			b.WriteString("\n")
		}
		b.WriteString(s.theme.Hint.Render("equivalent CLI: " + s.CLI()))
		b.WriteString("\n")
		b.WriteString(s.theme.Hint.Render("enter confirm · esc back"))
		return tea.NewView(b.String())
	}
	cli := s.cli
	if cli == "" {
		cli = s.CLI()
	}
	b.WriteString(s.theme.Hint.Render("equivalent CLI: " + cli))
	b.WriteString("\n\n")
	b.WriteString(s.log.View().Content)
	if s.stage == runtimeDone {
		b.WriteString("\n")
		b.WriteString(s.theme.Hint.Render("esc back"))
	}
	return tea.NewView(b.String())
}

// ServeScreen runs the HTTP server entrypoint behind a form→log pane.
type ServeScreen struct{ runtimeLauncher }

// NewServeScreen returns a serve screen over the project defaults.
func NewServeScreen() *ServeScreen {
	project, _ := runtimeLoadProject()
	return &ServeScreen{newRuntimeLauncher(
		"serve", "zever serve", "",
		"Server entry", "package directory run via go run",
		project.withDefaults().ServerEntry, project.withDefaults(),
	)}
}

// CLI returns the equivalent CLI string. Part of the screen contract.
func (s *ServeScreen) CLI() string { return s.runtimeLauncher.CLI() }

// Stop terminates the child. Part of the screen contract.
func (s *ServeScreen) Stop() { s.runtimeLauncher.Stop() }

// Init implements tea.Model.
func (s *ServeScreen) Init() tea.Cmd { return s.runtimeLauncher.Init() }

// Update implements tea.Model.
func (s *ServeScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return s, s.update(msg)
}

// View implements tea.Model.
func (s *ServeScreen) View() tea.View { return s.view() }

// QueueWorkScreen runs the worker entrypoint. It is also the schedule:run
// screen: the scheduler lives inside the worker process, so the alias needs
// no separate screen or entrypoint.
type QueueWorkScreen struct{ runtimeLauncher }

// NewQueueWorkScreen returns a worker screen over the project defaults.
func NewQueueWorkScreen() *QueueWorkScreen {
	project, _ := runtimeLoadProject()
	return &QueueWorkScreen{newRuntimeLauncher(
		"queue:work", "zever queue:work", "alias: zever schedule:run runs this same worker entrypoint",
		"Worker entry", "package directory run via go run",
		project.withDefaults().WorkerEntry, project.withDefaults(),
	)}
}

// CLI returns the equivalent CLI string. Part of the screen contract.
func (s *QueueWorkScreen) CLI() string { return s.runtimeLauncher.CLI() }

// Stop terminates the child. Part of the screen contract.
func (s *QueueWorkScreen) Stop() { s.runtimeLauncher.Stop() }

// Init implements tea.Model.
func (s *QueueWorkScreen) Init() tea.Cmd { return s.runtimeLauncher.Init() }

// Update implements tea.Model.
func (s *QueueWorkScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return s, s.update(msg)
}

// View implements tea.Model.
func (s *QueueWorkScreen) View() tea.View { return s.view() }

// --- dev screen ---

// runtimeDevLoop is the dev watch loop. Seam so tests drive it with a stub.
var runtimeDevLoop = devLoop

// DevScreen runs the file watcher behind a form→log pane. Debounce is an
// advanced field defaulting to the core devDebounce.
type DevScreen struct {
	stage    runtimeStage
	form     *huh.Form
	project  ProjectConfig
	entry    string
	extra    string
	debounce string
	prevDeb  time.Duration
	cli      string
	log      tui.LogModel
	cancel   context.CancelFunc
	pipeR    *io.PipeReader
	reader   *bufio.Reader
	canceled bool
	theme    tui.Theme
	keys     tui.Keymap
	width    int
	height   int
}

// NewDevScreen returns a dev screen over the project defaults.
func NewDevScreen() *DevScreen {
	project, _ := runtimeLoadProject()
	project = project.withDefaults()
	s := &DevScreen{
		project: project, entry: project.ServerEntry,
		debounce: strconv.FormatInt(devDebounce.Milliseconds(), 10),
		theme:    tui.NewTheme(), keys: tui.DefaultKeymap(), log: tui.NewLog(80, 18),
	}
	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Server entry").Description("package directory run via go run").Placeholder(defaultServerEntry).Value(&s.entry),
			huh.NewInput().Title("Extra args").Description("passed through untouched").Placeholder("-addr :9090").Value(&s.extra),
			huh.NewInput().Title("Debounce (ms)").Description("advanced: coalesces a burst of file events").Placeholder("250").Value(&s.debounce),
		),
	).WithLayout(huh.LayoutStack).WithWidth(60)
	return s
}

// resolvedEntry returns the form entry, falling back to the project default.
func (s *DevScreen) resolvedEntry() string {
	if e := strings.TrimSpace(s.entry); e != "" {
		return e
	}
	return s.project.ServerEntry
}

// entryValue reports the effective entry (test seam).
func (s *DevScreen) entryValue() string { return s.resolvedEntry() }

// debounceValue parses the advanced debounce field, defaulting to the core
// devDebounce on empty/invalid input.
func (s *DevScreen) debounceValue() time.Duration {
	if ms, err := strconv.Atoi(strings.TrimSpace(s.debounce)); err == nil && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return devDebounce
}

// CLI returns the equivalent CLI string from the current form values.
func (s *DevScreen) CLI() string {
	cli := "zever dev"
	if args := splitExtraArgs(s.extra); len(args) > 0 {
		cli += " " + strings.Join(args, " ")
	}
	return cli
}

// Init implements tea.Model.
func (s *DevScreen) Init() tea.Cmd { return s.form.Init() }

// startRunning launches devLoop under a cancellable context and enters the
// log pane. The loop owns the server child: cancelling the context unwinds
// the loop, whose deferred child.stop reaps the server (no orphans). The
// debounce override applies to the core tunables for the session and is
// restored on stop. Watcher diagnostics stream into the pane; the server
// child itself inherits stdio per the core startLaunch design.
func (s *DevScreen) startRunning() tea.Cmd {
	args := splitExtraArgs(s.extra)
	entry := s.resolvedEntry()
	s.cli = s.CLI()
	s.prevDeb = devDebounce
	devDebounce = s.debounceValue()
	proj := s.project
	proj.ServerEntry = entry
	r, w := io.Pipe()
	s.pipeR = r
	s.reader = bufio.NewReader(r)
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.stage = runtimeRunning
	s.log.Append("$ " + s.cli)
	return tea.Batch(
		func() tea.Msg {
			err := runtimeDevLoop(ctx, proj, args, w)
			_ = w.Close()
			_ = err // diagnostics already went to the pane; exit note below
			return runtimeDoneMsg{}
		},
		runtimeReadCmd(s.reader),
	)
}

// stopDev cancels the watch loop, restores the core debounce, and unblocks
// the loop's final writes so its deferred child.stop can run to completion.
func (s *DevScreen) stopDev() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	devDebounce = s.prevDeb
	if s.pipeR != nil {
		_ = s.pipeR.Close()
		s.pipeR = nil
	}
}

// updateForm forwards msg to the huh form and advances on completion.
func (s *DevScreen) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := s.form.Update(msg)
	switch s.form.State {
	case huh.StateCompleted:
		return s, s.startRunning()
	case huh.StateAborted:
		s.canceled = true
		return s, func() tea.Msg { return tui.CanceledMsg{} }
	default:
		return s, cmd
	}
}

// Update implements tea.Model. Esc in the pane cancels the context (the
// loop's deferred stop reaps the child) and pops immediately; the loop's
// own completion lands as runtimeDoneMsg and is then a no-op.
func (s *DevScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			s.width = msg.Width
			s.form = s.form.WithWidth(max(s.width-4, 20))
		}
		if msg.Height > 0 {
			s.height = msg.Height
		}
		lm, cmd := s.log.Update(msg)
		if l, ok := lm.(*tui.LogModel); ok {
			s.log = *l
		}
		return s, cmd
	case tea.KeyPressMsg:
		if s.keys.Back.Matches(msg.String()) {
			if s.stage == runtimeForm {
				s.canceled = true
				return s, func() tea.Msg { return tui.CanceledMsg{} }
			}
			return s, func() tea.Msg { s.stopDev(); return tui.LogBackMsg{} }
		}
		if s.stage == runtimeForm {
			return s.updateForm(msg)
		}
		lm, cmd := s.log.Update(msg)
		if l, ok := lm.(*tui.LogModel); ok {
			s.log = *l
		}
		return s, cmd
	case runtimeLineMsg:
		if s.stage != runtimeRunning {
			return s, nil
		}
		s.log.Append(msg.line)
		if s.reader == nil {
			return s, nil
		}
		return s, runtimeReadCmd(s.reader)
	case runtimeDoneMsg:
		if s.stage == runtimeRunning {
			s.stage = runtimeDone
			s.log.Append("exit: watch loop ended (esc back)")
		}
		return s, nil
	default:
		if s.stage == runtimeForm {
			return s.updateForm(msg)
		}
		return s, nil
	}
}

// View implements tea.Model. Pure.
func (s *DevScreen) View() tea.View {
	var b strings.Builder
	b.WriteString(s.theme.Title.Render("dev"))
	b.WriteString("\n\n")
	if s.stage == runtimeForm {
		b.WriteString(s.form.View())
		b.WriteString("\n\n")
		b.WriteString(s.theme.Hint.Render("equivalent CLI: " + s.CLI()))
		b.WriteString("\n")
		b.WriteString(s.theme.Hint.Render("enter confirm · esc back"))
		return tea.NewView(b.String())
	}
	cli := s.cli
	if cli == "" {
		cli = s.CLI()
	}
	b.WriteString(s.theme.Hint.Render("equivalent CLI: " + cli))
	b.WriteString("\n\n")
	b.WriteString(s.log.View().Content)
	if s.stage == runtimeDone {
		b.WriteString("\n")
		b.WriteString(s.theme.Hint.Render("esc back"))
	}
	return tea.NewView(b.String())
}

// --- tinker screen ---

// runtimeStartTinker starts the shim, pings it ready, and builds the yaegi
// interpreter over the shim-backed values. Seam so tests inject an
// interpreter without spawning the Go toolchain.
var runtimeStartTinker = defaultStartTinker

// defaultStartTinker is the production runtimeStartTinker.
//
// coverageSeam vars below keep behavior identical while letting tests stub
// shim start/ping/interp without spawning the Go toolchain; without them the
// interp-error and success returns are unreachable in `go test`, blocking
// 100% statement coverage. Proof: huh/yaegi prelude is fixed and always
// succeeds, and a real shim needs `go run` + protocol speaker.
var (
	tinkerShimStartForScreen = startTinkerShim
	tinkerInterpNewForScreen = newTinkerInterp
)

func defaultStartTinker(entry string, timeout time.Duration, passthrough io.Writer) (*interp.Interpreter, func() error, error) {
	client, err := tinkerShimStartForScreen(entry, passthrough)
	if err != nil {
		return nil, nil, err
	}
	if pingErr := client.ping(timeout); pingErr != nil {
		_ = client.Close()
		return nil, nil, pingErr
	}
	i, err := tinkerInterpNewForScreen(client)
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return i, client.Close, nil
}

// TinkerScreen hosts the yaegi REPL in a log pane. Evaluation goes through
// the core allowlist (tinkerAllowedSymbols via newTinkerInterp) with the
// screen's EvalTimeout from TinkerConfig; the entry passes through
// resolveTinkerConfig unchanged, so no symbol widening is possible.
type TinkerScreen struct {
	stage        runtimeStage
	form         *huh.Form
	project      ProjectConfig
	entry        string
	cli          string
	log          tui.LogModel
	interp       *interp.Interpreter
	cleanup      func() error
	input        string
	evalTimeout  time.Duration
	startTimeout time.Duration
	shimReader   *bufio.Reader
	pipeW        *io.PipeWriter
	pipeR        *io.PipeReader
	canceled     bool
	theme        tui.Theme
	keys         tui.Keymap
	width        int
	height       int
}

// NewTinkerScreen returns a tinker screen over the project defaults.
func NewTinkerScreen() *TinkerScreen {
	project, _ := runtimeLoadProject()
	project = project.withDefaults()
	s := &TinkerScreen{
		project: project, entry: project.TinkerEntry,
		evalTimeout: tinkerEvalTimeout, startTimeout: tinkerStartTimeout,
		theme: tui.NewTheme(), keys: tui.DefaultKeymap(), log: tui.NewLog(80, 18),
	}
	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Tinker shim entry").Description("shim package run via go run").Placeholder(defaultTinkerEntry).Value(&s.entry),
		),
	).WithLayout(huh.LayoutStack).WithWidth(60)
	return s
}

// resolvedEntry returns the form entry, falling back to the project default.
func (s *TinkerScreen) resolvedEntry() string {
	if e := strings.TrimSpace(s.entry); e != "" {
		return e
	}
	return s.project.TinkerEntry
}

// entryValue reports the effective entry (test seam).
func (s *TinkerScreen) entryValue() string { return s.resolvedEntry() }

// evalTimeoutValue reports the EvalTimeout (test seam).
func (s *TinkerScreen) evalTimeoutValue() time.Duration { return s.evalTimeout }

// startTimeoutValue reports the StartTimeout (test seam).
func (s *TinkerScreen) startTimeoutValue() time.Duration { return s.startTimeout }

// CLI returns the equivalent CLI string.
func (s *TinkerScreen) CLI() string { return "zever tinker --entry " + s.resolvedEntry() }

// Init implements tea.Model.
func (s *TinkerScreen) Init() tea.Cmd { return s.form.Init() }

// startRunning starts the shim and enters the REPL pane. The dev-only
// warning banners the pane before anything evaluates.
func (s *TinkerScreen) startRunning() tea.Cmd {
	entry := s.resolvedEntry()
	s.cli = s.CLI()
	r, w := io.Pipe()
	s.pipeR = r
	s.pipeW = w
	s.shimReader = bufio.NewReader(r)
	i, cleanup, err := runtimeStartTinker(entry, s.startTimeout, w)
	if err != nil {
		_ = w.Close()
		_ = r.Close()
		s.stage = runtimeDone
		s.log.Append("$ "+s.cli, "error: "+err.Error())
		return nil
	}
	s.interp = i
	s.cleanup = cleanup
	s.stage = runtimeRunning
	s.log.Append(tinkerDevWarning, "", "$ "+s.cli, tinkerBanner)
	return runtimeReadCmd(s.shimReader)
}

// closeTinker tears the shim down: pipes close, the shim context cancels,
// and the subprocess is Waited (no orphans). Safe with no session.
func (s *TinkerScreen) closeTinker() {
	if s.pipeW != nil {
		_ = s.pipeW.Close()
		s.pipeW = nil
	}
	if s.pipeR != nil {
		_ = s.pipeR.Close()
		s.pipeR = nil
	}
	if s.cleanup != nil {
		_ = s.cleanup()
		s.cleanup = nil
	}
	s.interp = nil
}

// evalErr evaluates one REPL line under the screen timeout, returning the
// raw error (test seam).
func (s *TinkerScreen) evalErr(src string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.evalTimeout)
	defer cancel()
	_, err := s.interp.EvalWithContext(ctx, normalizeTinkerSource(src))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("zever tinker: evaluation timed out after %s: %w", s.evalTimeout, err)
		}
		return err
	}
	return nil
}

// evalLine evaluates one REPL line under the screen timeout and renders the
// result for the pane.
func (s *TinkerScreen) evalLine(src string) string {
	ctx, cancel := context.WithTimeout(context.Background(), s.evalTimeout)
	defer cancel()
	v, err := s.interp.EvalWithContext(ctx, normalizeTinkerSource(src))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("error: evaluation timed out after %s", s.evalTimeout)
		}
		return "error: " + err.Error()
	}
	return formatTinkerValue(v)
}

// submitLine evaluates the input buffer and appends the exchange to the pane.
func (s *TinkerScreen) submitLine() {
	line := strings.TrimSpace(s.input)
	s.input = ""
	switch line {
	case "":
		return
	case ":exit", ":quit", "exit", "quit":
		s.closeTinker()
		s.stage = runtimeDone
		s.log.Append("zever> "+line, "bye.")
		return
	case ":help":
		s.log.Append("zever> :help", tinkerBanner)
		return
	}
	s.log.Append("zever> "+line, s.evalLine(line))
}

// updateForm forwards msg to the huh form and advances on completion.
func (s *TinkerScreen) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := s.form.Update(msg)
	switch s.form.State {
	case huh.StateCompleted:
		return s, s.startRunning()
	case huh.StateAborted:
		s.canceled = true
		return s, func() tea.Msg { return tui.CanceledMsg{} }
	default:
		return s, cmd
	}
}

// appendRune adds typed text to the REPL input buffer.
func (s *TinkerScreen) appendRune(text string) {
	s.input += text
}

// backspace drops the last rune from the REPL input buffer.
func (s *TinkerScreen) backspace() {
	if s.input == "" {
		return
	}
	r := []rune(s.input)
	s.input = string(r[:len(r)-1])
}

// Update implements tea.Model. In the REPL pane, printable keys edit the
// input buffer, enter submits, and esc closes the shim and pops.
func (s *TinkerScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			s.width = msg.Width
			s.form = s.form.WithWidth(max(s.width-4, 20))
		}
		if msg.Height > 0 {
			s.height = msg.Height
		}
		lm, cmd := s.log.Update(msg)
		if l, ok := lm.(*tui.LogModel); ok {
			s.log = *l
		}
		return s, cmd
	case tea.KeyPressMsg:
		if s.keys.Back.Matches(msg.String()) {
			if s.stage == runtimeForm {
				s.canceled = true
				return s, func() tea.Msg { return tui.CanceledMsg{} }
			}
			return s, func() tea.Msg { s.closeTinker(); return tui.LogBackMsg{} }
		}
		if s.stage == runtimeForm {
			return s.updateForm(msg)
		}
		if s.stage != runtimeRunning {
			lm, cmd := s.log.Update(msg)
			if l, ok := lm.(*tui.LogModel); ok {
				s.log = *l
			}
			return s, cmd
		}
		switch msg.String() {
		case "enter":
			s.submitLine()
			if s.stage == runtimeDone {
				return s, func() tea.Msg { return tui.LogBackMsg{} }
			}
			return s, nil
		case "backspace":
			s.backspace()
			return s, nil
		default:
			if msg.Text != "" && msg.Mod == 0 {
				s.appendRune(msg.Text)
			} else {
				lm, cmd := s.log.Update(msg)
				if l, ok := lm.(*tui.LogModel); ok {
					s.log = *l
				}
				return s, cmd
			}
			return s, nil
		}
	case runtimeLineMsg:
		if s.stage != runtimeRunning {
			return s, nil
		}
		s.log.Append(msg.line)
		if s.shimReader == nil {
			return s, nil
		}
		return s, runtimeReadCmd(s.shimReader)
	case runtimeDoneMsg:
		if s.stage == runtimeRunning {
			s.stage = runtimeDone
			s.log.Append("exit: shim output ended (esc back)")
		}
		return s, nil
	default:
		if s.stage == runtimeForm {
			return s.updateForm(msg)
		}
		return s, nil
	}
}

// View implements tea.Model. Pure. The form phase leads with the dev-only
// warning so it shows before anything starts.
func (s *TinkerScreen) View() tea.View {
	var b strings.Builder
	b.WriteString(s.theme.Title.Render("tinker"))
	b.WriteString("\n\n")
	if s.stage == runtimeForm {
		b.WriteString(s.theme.Fail.Render(tinkerDevWarning))
		b.WriteString("\n\n")
		b.WriteString(s.form.View())
		b.WriteString("\n\n")
		b.WriteString(s.theme.Hint.Render("equivalent CLI: " + s.CLI()))
		b.WriteString("\n")
		b.WriteString(s.theme.Hint.Render("enter confirm · esc back"))
		return tea.NewView(b.String())
	}
	cli := s.cli
	if cli == "" {
		cli = s.CLI()
	}
	b.WriteString(s.theme.Hint.Render("equivalent CLI: " + cli))
	b.WriteString("\n\n")
	b.WriteString(s.log.View().Content)
	b.WriteString("\n")
	if s.stage == runtimeRunning {
		b.WriteString("zever> " + s.input + "▌")
	} else {
		b.WriteString(s.theme.Hint.Render("esc back"))
	}
	return tea.NewView(b.String())
}
