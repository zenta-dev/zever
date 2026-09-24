package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// devBrokenSchema is not valid zen: the resolver must reject it, which is the
// trigger for the "do not restart a working server" branch. The valid-schema
// counterpart is the sibling-owned zeverUserSchema helper.
const devBrokenSchema = `entity User {
	id: uuid @primary
	bad_field: nonexistent_type
}
`

// syncBuffer is a bytes.Buffer safe for concurrent use while devLoop writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

func mkdirZeverAll(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// TestDevCompileReportsBrokenSchema pins the compile step dev restarts off:
// diagnostics surface as an error, not swallowed.
func TestDevCompileReportsBrokenSchema(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schema")

	if err := mkdirZeverAll(schemaDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeZeverFixture(t, schemaDir, "user.zen", devBrokenSchema)

	if err := devCompile(schemaDir, &syncBuffer{}); err == nil {
		t.Fatal("devCompile accepted a schema that does not resolve")
	}

	writeZeverFixture(t, schemaDir, "user.zen", zeverUserSchema)

	if err := devCompile(schemaDir, &syncBuffer{}); err != nil {
		t.Fatalf("devCompile on a valid schema: %v", err)
	}
}

// TestDevCompileEmptySchemaDir proves a project with no schemas yet is not an
// error: dev still starts and watches.
func TestDevCompileEmptySchemaDir(t *testing.T) {
	if err := devCompile(filepath.Join(t.TempDir(), "absent"), &syncBuffer{}); err != nil {
		t.Fatalf("devCompile on a missing schema dir: %v", err)
	}
}

// TestRunDevHelp proves the help path never touches the filesystem or the go
// toolchain.
func TestRunDevHelp(t *testing.T) {
	if err := runDev([]string{"-h"}); err != nil {
		t.Fatalf("runDev -h: %v", err)
	}
}

// devStartRecord is a mutex-guarded record of dev child starts. devLoop
// appends from its own goroutine while tests poll from theirs, so a plain
// slice races under -race (write vs len). All access goes through the
// methods below; tests must never touch entries directly.
type devStartRecord struct {
	mu      sync.Mutex
	entries []string
}

func (r *devStartRecord) add(entry string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
}

func (r *devStartRecord) numStarts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// stubDevSeams replaces the dev subprocess seams with fakes and returns the
// fake child factory's call record.
func stubDevSeams(t *testing.T, watcher *fsnotify.Watcher) *devStartRecord {
	t.Helper()

	starts := &devStartRecord{}
	fake := &devChild{cmd: nil, done: make(chan struct{})}
	close(fake.done)

	origStart := devStartChild
	origWatch := devNewWatcher
	origCompile := devCompileFunc

	devStartChild = func(entryDir string, _ []string, _ io.Writer) (*devChild, error) {
		starts.add(entryDir)
		return fake, nil
	}
	devNewWatcher = func(ProjectConfig) (*fsnotify.Watcher, error) {
		return watcher, nil
	}
	devCompileFunc = func(string, io.Writer) error {
		return nil
	}

	t.Cleanup(func() {
		devStartChild = origStart
		devNewWatcher = origWatch
		devCompileFunc = origCompile
	})

	return starts
}

// TestDevLoopStopsOnCancel proves the loop unwinds without a subprocess:
// seams stubbed, context cancelled, no sleeps.
func TestDevLoopStopsOnCancel(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	starts := stubDevSeams(t, watcher)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	out := &syncBuffer{}
	project := ProjectConfig{SchemaDir: "schema", ServerEntry: "cmd/server"}.withDefaults()

	if err := devLoop(ctx, project, nil, out); err != nil {
		t.Fatalf("devLoop: %v", err)
	}

	if starts.numStarts() != 1 {
		t.Fatalf("expected one child start, got %d", starts.numStarts())
	}

	if !strings.Contains(out.String(), "watching") {
		t.Fatalf("expected watching banner, got:\n%s", out.String())
	}
}

// TestDevLoopDebounceUsesTimerSeam proves the debounce goes through the
// injectable timer constructor with the configured interval.
func TestDevLoopDebounceUsesTimerSeam(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	starts := stubDevSeams(t, watcher)

	var saw time.Duration
	origTimer := devNewTimer
	devNewTimer = func(d time.Duration) *time.Timer {
		saw = d
		return origTimer(d)
	}
	t.Cleanup(func() { devNewTimer = origTimer })

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	project := ProjectConfig{SchemaDir: "schema", ServerEntry: "cmd/server"}.withDefaults()

	if err := devLoop(ctx, project, nil, &syncBuffer{}); err != nil {
		t.Fatalf("devLoop: %v", err)
	}

	if starts.numStarts() != 1 {
		t.Fatalf("expected one child start, got %d", starts.numStarts())
	}

	if saw != devDebounce {
		t.Fatalf("debounce timer interval = %v, want %v", saw, devDebounce)
	}
}

// TestDevRebuildKeepsChildOnCompileFailure proves a broken edit never costs
// the running server: no restart, same child returned.
func TestDevRebuildKeepsChildOnCompileFailure(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schema")

	if err := mkdirZeverAll(schemaDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeZeverFixture(t, schemaDir, "user.zen", devBrokenSchema)

	running := &devChild{done: make(chan struct{})}
	close(running.done)

	project := ProjectConfig{SchemaDir: schemaDir, ServerEntry: "cmd/server"}.withDefaults()
	out := &syncBuffer{}

	if got := devRebuild(project, nil, out, running); got != running {
		t.Fatal("failed compile must keep the running child")
	}

	if !strings.Contains(out.String(), "server unchanged") {
		t.Fatalf("expected server-unchanged report, got:\n%s", out.String())
	}
}

// TestDevLoopEndToEnd is the full proof: a genuine fsnotify watcher, a
// genuine `go run` child, a genuine file edit. Skipped in short mode.
func TestDevLoopEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles and runs a child program via `go run`, and watches the filesystem")
	}

	origDebounce, origGrace := devDebounce, devStopGrace
	devDebounce, devStopGrace = 100*time.Millisecond, 3*time.Second
	t.Cleanup(func() { devDebounce, devStopGrace = origDebounce, origGrace })

	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schema")
	serverDir := filepath.Join(dir, "cmd", "server")

	if err := mkdirZeverAll(schemaDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := mkdirZeverAll(serverDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeZeverFixture(t, dir, "go.mod", "module devtest\n\ngo 1.24\n")
	writeZeverFixture(t, serverDir, "main.go", devChildProgram)
	writeZeverFixture(t, schemaDir, "user.zen", zeverUserSchema)

	logPath := filepath.Join(dir, "lifecycle.log")

	t.Chdir(dir)

	project := ProjectConfig{}.withDefaults()

	out := &syncBuffer{}
	ctx, cancel := context.WithCancel(t.Context())

	errs := make(chan error, 1)

	go func() { errs <- devLoop(ctx, project, []string{logPath}, out) }()

	defer func() {
		cancel()

		select {
		case err := <-errs:
			if err != nil {
				t.Errorf("devLoop: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("devLoop did not return after its context was cancelled")
		}
	}()

	waitForZeverLifecycle(t, logPath, []string{"start"}, 120*time.Second)

	writeZeverFixture(t, schemaDir, "user.zen", zeverEditedSchema)
	waitForZeverLifecycle(t, logPath, []string{"start", "term", "start"}, 120*time.Second)

	writeZeverFixture(t, schemaDir, "user.zen", devBrokenSchema)

	// Poll for the rebuild outcome instead of a fixed 3s sleep: the failed
	// compile is reported on out, which proves the debounced rebuild ran
	// and rejected the edit. The lifecycle must then still show exactly
	// [start term start] (server left untouched).
	pollFor(t, 30*time.Second, func() bool {
		return strings.Contains(out.String(), "compile FAILED, server unchanged")
	})

	if got := readZeverLifecycle(t, logPath); len(got) != 3 {
		t.Fatalf("lifecycle after a broken edit = %v, want the server left untouched at [start term start]", got)
	}

	if !strings.Contains(out.String(), "compile FAILED, server unchanged") {
		t.Fatalf("dev output did not report the failed compile:\n%s", out.String())
	}

	cancel()
	waitForZeverLifecycle(t, logPath, []string{"start", "term", "start", "term"}, 30*time.Second)
}

// devChildProgram is the fixture server the dev integration test runs through
// the real launcher. Every incarnation appends "start" to a shared log on
// boot and "term" when asked to shut down.
const devChildProgram = `package main

import (
	"os"
	"os/signal"
	"syscall"
)

func appendLine(path, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}

	if _, err := f.WriteString(line + "\n"); err != nil {
		panic(err)
	}

	if err := f.Close(); err != nil {
		panic(err)
	}
}

func main() {
	log := os.Args[1]

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	appendLine(log, "start")
	<-ch
	appendLine(log, "term")
}
`

// zeverEditedSchema is a valid change: compiling it succeeds, so a restart
// is expected.
const zeverEditedSchema = `entity User {
	id: uuid @primary
	email: string @unique
	name: string
}

service UserService {
	rpc GetUser(id: uuid) -> User {
		http: GET "/v1/users/{id}"
		auth: required
	}
}
`

func readZeverLifecycle(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // test-controlled path
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		t.Fatalf("read lifecycle log: %v", err)
	}

	return strings.Fields(string(data))
}

func waitForZeverLifecycle(t *testing.T, path string, want []string, within time.Duration) {
	t.Helper()

	deadline := time.Now().Add(within)

	var got []string

	for time.Now().Before(deadline) {
		got = readZeverLifecycle(t, path)
		if strings.Join(got, ",") == strings.Join(want, ",") {
			return
		}

		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("test context done: lifecycle = %v, want %v", got, want)
		case <-timer.C:
		}
	}

	t.Fatalf("timed out after %s: lifecycle = %v, want %v", within, got, want)
}

// TestDevCompileRejectsFile pins the not-a-directory error branch.
func TestDevCompileRejectsFile(t *testing.T) {
	dir := t.TempDir()
	file := writeZeverFixture(t, dir, "app.zen", zeverUserSchema)

	if err := devCompile(file, &syncBuffer{}); err == nil {
		t.Fatal("expected an error for a file schema dir, got nil")
	}
}

// TestStartDevChildReportsLaunchFailure pins the spawn error branch by hiding
// the go toolchain from PATH.
func TestStartDevChildReportsLaunchFailure(t *testing.T) {
	orig := os.Getenv("PATH")
	t.Setenv("PATH", "")

	_ = orig

	if _, err := startDevChild("cmd/server", nil, &syncBuffer{}); err == nil {
		t.Fatal("expected a launch error without go on PATH, got nil")
	}
}

// TestDevChildStopKillsWedgedChild proves the grace expiry path: a child that
// ignores SIGTERM is SIGKILLed so a restart never overlaps the old server.
// Needs the go toolchain, so it skips in short mode.
func TestDevChildStopKillsWedgedChild(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles and runs a child program via `go run`")
	}

	dir := t.TempDir()
	progDir := filepath.Join(dir, "prog")

	if err := mkdirZeverAll(progDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeZeverFixture(t, dir, "go.mod", "module wedgetest\n\ngo 1.24\n")
	writeZeverFixture(t, progDir, "main.go", "package main\n\nimport (\n\t\"os\"\n\t\"os/signal\"\n\t\"time\"\n)\n\nfunc main() {\n\tch := make(chan os.Signal, 1)\n\tsignal.Notify(ch)\n\t_ = ch\n\ttime.Sleep(10 * time.Minute)\n}\n")

	t.Chdir(dir)

	child, err := startDevChild("./prog", nil, &syncBuffer{})
	if err != nil {
		t.Fatalf("startDevChild: %v", err)
	}

	done := make(chan struct{})

	go func() {
		child.stop(200 * time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("stop did not return after killing the wedged child")
	}

	select {
	case <-child.done:
	default:
		t.Fatal("child was not reaped after stop")
	}
}

// TestDevLoopClosedWatcher proves a dead watcher channel ends the loop
// without touching a timer: the debounce is parked for an hour so the only
// ready case is the closed channel.
func TestDevLoopClosedWatcher(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	if err := watcher.Close(); err != nil {
		t.Fatalf("pre-close watcher: %v", err)
	}

	stubClosedDevSeams(t, watcher)

	origDebounce := devDebounce
	devDebounce = time.Hour
	t.Cleanup(func() { devDebounce = origDebounce })

	ctx := t.Context()
	project := ProjectConfig{SchemaDir: "schema", ServerEntry: "cmd/server"}.withDefaults()

	if err := devLoop(ctx, project, nil, &syncBuffer{}); err != nil {
		t.Fatalf("devLoop: %v", err)
	}
}

// stubClosedDevSeams stubs the dev seams for tests that never fire the timer.
func stubClosedDevSeams(t *testing.T, watcher *fsnotify.Watcher) *devStartRecord {
	t.Helper()

	starts := &devStartRecord{}
	fake := &devChild{done: make(chan struct{})}
	close(fake.done)

	origStart := devStartChild
	origWatch := devNewWatcher
	origCompile := devCompileFunc

	devStartChild = func(string, []string, io.Writer) (*devChild, error) {
		starts.add("cmd/server")
		return fake, nil
	}
	devNewWatcher = func(ProjectConfig) (*fsnotify.Watcher, error) {
		return watcher, nil
	}
	devCompileFunc = func(string, io.Writer) error { return nil }

	t.Cleanup(func() {
		devStartChild = origStart
		devNewWatcher = origWatch
		devCompileFunc = origCompile
	})

	return starts
}

// TestDevLoopReportsWatchError proves watcher failures are printed and
// survived: inject one, then cancel.
func TestDevLoopReportsWatchError(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	stubClosedDevSeams(t, watcher)

	origDebounce := devDebounce
	devDebounce = time.Hour
	t.Cleanup(func() { devDebounce = origDebounce })

	ctx, cancel := context.WithCancel(t.Context())

	errs := make(chan error, 1)
	out := &syncBuffer{}
	project := ProjectConfig{SchemaDir: "schema", ServerEntry: "cmd/server"}.withDefaults()

	go func() { errs <- devLoop(ctx, project, nil, out) }()

	watcher.Errors <- errTestSentinel

	cancel()

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("devLoop: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("devLoop did not return after cancel")
	}

	if !strings.Contains(out.String(), "watch error:") {
		t.Fatalf("expected watch error report, got:\n%s", out.String())
	}
}

// TestDevLoopRebuildsOnCreate proves the full event path with polling instead
// of sleeps: mkdir in the schema tree triggers exactly one debounced rebuild
// that restarts the server.
func TestDevLoopRebuildsOnCreate(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schema")
	entryDir := filepath.Join(dir, "cmd", "server")

	if err := mkdirZeverAll(schemaDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := mkdirZeverAll(entryDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Chdir(dir)

	watcher, err := newDevWatcher(ProjectConfig{SchemaDir: schemaDir, ServerEntry: entryDir}.withDefaults())
	if err != nil {
		t.Fatalf("newDevWatcher: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	starts := stubDevSeams(t, watcher)

	origDebounce := devDebounce
	devDebounce = 5 * time.Millisecond
	t.Cleanup(func() { devDebounce = origDebounce })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errs := make(chan error, 1)
	out := &syncBuffer{}
	project := ProjectConfig{SchemaDir: schemaDir, ServerEntry: entryDir}.withDefaults()

	go func() { errs <- devLoop(ctx, project, nil, out) }()

	pollFor(t, 10*time.Second, func() bool {
		return strings.Contains(out.String(), "watching")
	})

	if err := mkdirZeverAll(filepath.Join(schemaDir, "newpkg")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	pollFor(t, 10*time.Second, func() bool {
		return strings.Contains(out.String(), "restarting server")
	})

	cancel()

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("devLoop: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("devLoop did not return after cancel")
	}

	if starts.numStarts() != 2 {
		t.Fatalf("expected initial start plus one restart, got %d", starts.numStarts())
	}
}

// pollFor blocks until cond holds, failing with a timeout instead of a fixed
// sleep, so watcher timing stays flake-free.
func pollFor(t *testing.T, within time.Duration, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(within)

	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("test context done waiting for condition")
		case <-timer.C:
		}
	}

	t.Fatalf("timed out after %s waiting for condition", within)
}

// TestRunDevStopsOnSignal proves runDev unwinds on SIGTERM with stubbed
// seams: the loop starts one child, the signal ends the session cleanly.
func TestRunDevStopsOnSignal(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schema")
	entryDir := filepath.Join(dir, "cmd", "server")

	if err := mkdirZeverAll(schemaDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := mkdirZeverAll(entryDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Chdir(dir)

	watcher, err := newDevWatcher(ProjectConfig{SchemaDir: schemaDir, ServerEntry: entryDir}.withDefaults())
	if err != nil {
		t.Fatalf("newDevWatcher: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	starts := stubDevSeams(t, watcher)

	origDebounce := devDebounce
	devDebounce = time.Hour
	t.Cleanup(func() { devDebounce = origDebounce })

	guard := make(chan os.Signal, 1)
	signal.Notify(guard, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(guard)

	errs := make(chan error, 1)

	go func() { errs <- runDev(nil) }()

	pollFor(t, 10*time.Second, func() bool { return starts.numStarts() == 1 })

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill self: %v", err)
	}

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("runDev: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runDev did not return after SIGTERM")
	}
}

// TestDevRebuildReportsRelaunchFailure proves a failed relaunch keeps the
// running child and reports the failure.
func TestDevRebuildReportsRelaunchFailure(t *testing.T) {
	origCompile := devCompileFunc
	origStart := devStartChild
	devCompileFunc = func(string, io.Writer) error { return nil }
	calls := 0
	devStartChild = func(string, []string, io.Writer) (*devChild, error) {
		calls++
		if calls == 1 {
			fake := &devChild{done: make(chan struct{})}
			close(fake.done)
			return fake, nil
		}
		return nil, errTestSentinel
	}
	t.Cleanup(func() { devCompileFunc = origCompile; devStartChild = origStart })

	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "schema")

	if err := mkdirZeverAll(schemaDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	project := ProjectConfig{SchemaDir: schemaDir, ServerEntry: "cmd/server"}.withDefaults()
	out := &syncBuffer{}

	running, err := devStartChild(project.ServerEntry, nil, out)
	if err != nil {
		t.Fatalf("initial start: %v", err)
	}

	if got := devRebuild(project, nil, out, running); got != running {
		t.Fatal("failed relaunch must keep the running child")
	}

	if !strings.Contains(out.String(), "relaunch failed") {
		t.Fatalf("expected relaunch failure report, got:\n%s", out.String())
	}
}

// TestRunDevBadProject pins the config error branch of runDev.
func TestRunDevBadProject(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  schema_dir: [unclosed\n")
	t.Chdir(dir)

	if err := runDev(nil); err == nil {
		t.Fatal("expected a config error, got nil")
	}
}

// TestAddWatchTreeUnreadable pins the walk error branch with an unreadable
// subdirectory. Skipped for root, for whom permissions do not apply.
func TestAddWatchTreeUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permission bits")
	}

	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")

	if err := mkdirZeverAll(filepath.Join(locked, "sub")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	defer func() { _ = watcher.Close() }()

	if err := addWatchTree(watcher, dir); err == nil {
		t.Fatal("expected a walk error, got nil")
	}
}

// TestDevLoopClosedWatcherRepeatedly runs the closed-watcher loop enough
// times to cover both channel-closed branches (Events and Errors race; each
// iteration takes one of them).
func TestDevLoopClosedWatcherRepeatedly(t *testing.T) {
	for i := 0; i < 20; i++ {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			t.Fatalf("new watcher: %v", err)
		}

		if err := watcher.Close(); err != nil {
			t.Fatalf("pre-close watcher: %v", err)
		}

		stubClosedDevSeams(t, watcher)

		origDebounce := devDebounce
		devDebounce = time.Hour
		func() {
			defer func() { devDebounce = origDebounce }()

			ctx := t.Context()
			project := ProjectConfig{SchemaDir: "schema", ServerEntry: "cmd/server"}.withDefaults()

			if err := devLoop(ctx, project, nil, &syncBuffer{}); err != nil {
				t.Fatalf("devLoop: %v", err)
			}
		}()
	}
}

// TestDevLoopSkipsIrrelevantEvent proves non-matching events reset nothing
// and the loop survives them: inject one directly, then cancel.
func TestDevLoopSkipsIrrelevantEvent(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	starts := stubClosedDevSeams(t, watcher)

	origDebounce := devDebounce
	devDebounce = time.Hour
	t.Cleanup(func() { devDebounce = origDebounce })

	ctx, cancel := context.WithCancel(t.Context())

	errs := make(chan error, 1)
	out := &syncBuffer{}
	project := ProjectConfig{SchemaDir: "schema", ServerEntry: "cmd/server"}.withDefaults()

	go func() { errs <- devLoop(ctx, project, nil, out) }()

	watcher.Events <- fsnotify.Event{Name: "README.md", Op: fsnotify.Write}

	cancel()

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("devLoop: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("devLoop did not return after cancel")
	}

	if starts.numStarts() != 1 {
		t.Fatalf("irrelevant event must not rebuild, starts = %d", starts.numStarts())
	}
}

// TestDevLoopWatcherError proves the watcher constructor error surfaces.
func TestDevLoopWatcherError(t *testing.T) {
	orig := devNewWatcher
	devNewWatcher = func(ProjectConfig) (*fsnotify.Watcher, error) {
		return nil, errTestSentinel
	}
	t.Cleanup(func() { devNewWatcher = orig })

	ctx := t.Context()
	project := ProjectConfig{SchemaDir: "schema", ServerEntry: "cmd/server"}.withDefaults()

	if err := devLoop(ctx, project, nil, &syncBuffer{}); err == nil {
		t.Fatal("expected a watcher error, got nil")
	}
}

// TestDevLoopStartError proves a failed initial launch surfaces.
func TestDevLoopStartError(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	origWatch := devNewWatcher
	origStart := devStartChild
	origCompile := devCompileFunc
	devNewWatcher = func(ProjectConfig) (*fsnotify.Watcher, error) { return watcher, nil }
	devStartChild = func(string, []string, io.Writer) (*devChild, error) { return nil, errTestSentinel }
	devCompileFunc = func(string, io.Writer) error { return nil }
	t.Cleanup(func() { devNewWatcher = origWatch; devStartChild = origStart; devCompileFunc = origCompile })

	ctx := t.Context()
	project := ProjectConfig{SchemaDir: "schema", ServerEntry: "cmd/server"}.withDefaults()

	if err := devLoop(ctx, project, nil, &syncBuffer{}); err == nil {
		t.Fatal("expected a start error, got nil")
	}
}

// TestDevLoopInitialCompileFailure proves the advisory first compile prints
// its diagnostics and still starts the server.
func TestDevLoopInitialCompileFailure(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	starts := stubClosedDevSeams(t, watcher)

	origCompile := devCompileFunc
	devCompileFunc = func(string, io.Writer) error { return errTestSentinel }
	t.Cleanup(func() { devCompileFunc = origCompile })

	origDebounce := devDebounce
	devDebounce = time.Hour
	t.Cleanup(func() { devDebounce = origDebounce })

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	out := &syncBuffer{}
	project := ProjectConfig{SchemaDir: "schema", ServerEntry: "cmd/server"}.withDefaults()

	if err := devLoop(ctx, project, nil, out); err != nil {
		t.Fatalf("devLoop: %v", err)
	}

	if starts.numStarts() != 1 {
		t.Fatalf("server must start despite the failed initial compile, starts = %d", starts.numStarts())
	}

	if !strings.Contains(out.String(), "starting the server anyway") {
		t.Fatalf("expected the advisory-compile notice, got:\n%s", out.String())
	}
}

// TestDevLoopDrainsFiredTimer pins the debounce setup branch where the fresh
// timer has already fired (e.g. a stubbed clock): the tick is drained so the
// first rebuild does not fire immediately.
func TestDevLoopDrainsFiredTimer(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	stubClosedDevSeams(t, watcher)

	origTimer := devNewTimer
	devNewTimer = func(_ time.Duration) *time.Timer {
		// Return an already-fired timer: block on its channel so the
		// stubbed clock is guaranteed expired without a fixed sleep.
		// devLoop discards Stop's result, so the consumed tick is fine.
		tm := time.NewTimer(time.Nanosecond)
		<-tm.C
		return tm
	}
	t.Cleanup(func() { devNewTimer = origTimer })

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	project := ProjectConfig{SchemaDir: "schema", ServerEntry: "cmd/server"}.withDefaults()

	if err := devLoop(ctx, project, nil, &syncBuffer{}); err != nil {
		t.Fatalf("devLoop: %v", err)
	}
}
