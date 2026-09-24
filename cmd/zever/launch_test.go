package main

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// signalChild is a throwaway program the signal-forwarding integration test
// compiles and runs through the real runLaunch. It announces readiness by
// creating a file, blocks until SIGINT/SIGTERM arrives, then records the
// signal in a sentinel file and exits 0 -- exactly the shape of the graceful
// shutdown path the reference entrypoints implement.
const signalChild = `package main

import (
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	if err := os.WriteFile(os.Args[1], []byte("ready"), 0o600); err != nil {
		panic(err)
	}

	sig := <-ch

	if err := os.WriteFile(os.Args[2], []byte(sig.String()), 0o600); err != nil {
		panic(err)
	}
}
`

// TestRunLaunchForwardsSignalToChild is the real integration test for
// launch.go: no stubbing, an actual `go run` child, an actual OS signal.
func TestRunLaunchForwardsSignalToChild(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles and runs a child program via `go run`")
	}

	dir := t.TempDir()
	progDir := filepath.Join(dir, "prog")

	if err := os.MkdirAll(progDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeZeverFixture(t, dir, "go.mod", "module launchtest\n\ngo 1.24\n")
	writeZeverFixture(t, progDir, "main.go", signalChild)

	readyPath := filepath.Join(dir, "ready")
	sentinelPath := filepath.Join(dir, "sentinel")

	// Keep the test process itself alive through the SIGINT below: without a
	// handler registered here there is a window before/after runLaunch's own
	// signal.Notify in which the default action would kill the test binary.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, os.Interrupt, syscall.SIGTERM)

	defer signal.Stop(guard)

	t.Chdir(dir)

	errs := make(chan error, 1)

	go func() {
		errs <- runLaunch("./prog", []string{readyPath, sentinelPath})
	}()

	waitForZeverFile(t, readyPath, 90*time.Second)

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("kill self with SIGINT: %v", err)
	}

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("runLaunch: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("runLaunch did not return after the child was signalled")
	}

	got, err := os.ReadFile(sentinelPath) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatalf("child never wrote the sentinel (signal not forwarded): %v", err)
	}

	if !strings.Contains(string(got), "interrupt") {
		t.Fatalf("sentinel = %q, want it to name the interrupt signal", got)
	}
}

// TestRunLaunchMissingEntry proves a bad entry directory surfaces as an error
// rather than a silent success.
func TestRunLaunchMissingEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes the go toolchain")
	}

	t.Chdir(t.TempDir())

	if err := runLaunch("./does-not-exist", nil); err == nil {
		t.Fatal("expected an error launching a nonexistent entry package, got nil")
	}
}

func waitForZeverFile(t *testing.T, path string, within time.Duration) {
	t.Helper()

	deadline := time.Now().Add(within)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}

		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("aborted waiting for %q: test context done", path)
		case <-timer.C:
		}
	}

	t.Fatalf("timed out after %s waiting for %q", within, path)
}

// TestGoRunPath pins the "./" prefixing: `go run cmd/server` makes the go tool
// look for a standard-library package of that name and fail, which is exactly
// what the default entry conventions would hit.
func TestGoRunPath(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"cmd/server":   "./cmd/server",
		"db/seed":      "./db/seed",
		"./cmd/worker": "./cmd/worker",
		"../shared/cd": "../shared/cd",
		".":            ".",
		"":             ".",
		"/abs/cmd/srv": "/abs/cmd/srv",
	}

	for in, want := range cases {
		if got := goRunPath(in); got != want {
			t.Errorf("goRunPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestBuildGoRunArgs pins the argv construction: no shell, arg arrays only.
func TestBuildGoRunArgs(t *testing.T) {
	t.Parallel()

	got := buildGoRunArgs("cmd/server", []string{"-addr", ":9090"})
	want := []string{"run", "./cmd/server", "-addr", ":9090"}

	if len(got) != len(want) {
		t.Fatalf("buildGoRunArgs = %#v, want %#v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("buildGoRunArgs = %#v, want %#v", got, want)
		}
	}
}

// TestDescShort pins every launcher short description plus the fallback.
func TestDescShort(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"zever serve":        "run server entrypoint",
		"zever queue:work":   "run worker entrypoint",
		"zever schedule:run": "run scheduler (alias for queue:work)",
		"zever db seed":      "seed database",
		"zever dev":          "watch & restart",
		"zever unknown":      "launcher",
	}

	for in, want := range cases {
		if got := descShort(in); got != want {
			t.Errorf("descShort(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestStartLaunchReportsMissingTool pins the spawn error branch by hiding
// the go toolchain from PATH: no child is ever created.
func TestStartLaunchReportsMissingTool(t *testing.T) {
	t.Setenv("PATH", "")

	if _, err := startLaunch("cmd/server", nil); err == nil {
		t.Fatal("expected a launch error without go on PATH, got nil")
	}
}

// TestSignalHelpersTouchLiveProcess pins forwardSignal's direct-signal
// fallback and killProcessGroup against a real short-lived sleeper.
// Skipped on Windows, which has neither sleep nor POSIX signals.
func TestSignalHelpersTouchLiveProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals only")
	}

	proc := exec.CommandContext(t.Context(), "sleep", "60")

	if err := proc.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}

	forwardSignal(proc, testOnlySignal{})

	killProcessGroup(proc)

	if err := proc.Wait(); err == nil {
		t.Fatal("expected the sleeper to be killed, got nil error")
	}
}

type testOnlySignal struct{}

func (testOnlySignal) String() string { return "test-only" }
func (testOnlySignal) Signal()        {}

// TestSignalHelpersNilProcess proves the nil-process guards never panic.
func TestSignalHelpersNilProcess(t *testing.T) {
	t.Parallel()

	empty := &exec.Cmd{}

	forwardSignal(empty, testOnlySignal{})
	killProcessGroup(empty)
}

// TestRunLaunchSuccess proves the clean-exit path returns nil: a program
// that exits 0 on its own needs no signal delivery.
func TestRunLaunchSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes the go toolchain")
	}

	dir := t.TempDir()
	progDir := filepath.Join(dir, "prog")

	if err := os.MkdirAll(progDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeZeverFixture(t, dir, "go.mod", "module quickexit\n\ngo 1.24\n")
	writeZeverFixture(t, progDir, "main.go", "package main\n\nfunc main() {}\n")

	t.Chdir(dir)

	if err := runLaunch("./prog", nil); err != nil {
		t.Fatalf("runLaunch: %v", err)
	}
}

// TestRunLaunchStartFailure proves a missing toolchain surfaces as an error
// instead of hanging.
func TestRunLaunchStartFailure(t *testing.T) {
	t.Setenv("PATH", "")

	if err := runLaunch("./prog", nil); err == nil {
		t.Fatal("expected a launch error without go on PATH, got nil")
	}
}

// TestForwardSignalFallsBack pins the direct-signal fallback: signalling an
// already-reaped process fails group delivery and degrades gracefully.
func TestForwardSignalFallsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals only")
	}

	proc := exec.CommandContext(t.Context(), "true")

	if err := proc.Start(); err != nil {
		t.Fatalf("start true: %v", err)
	}

	if err := proc.Wait(); err != nil {
		t.Fatalf("wait true: %v", err)
	}

	forwardSignal(proc, syscall.SIGTERM)
}
