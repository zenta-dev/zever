package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// resetInteractiveMode restores the global after tests that peel -i flags.
func resetInteractiveMode(t *testing.T) {
	t.Helper()
	old := interactiveMode
	t.Cleanup(func() { interactiveMode = old })
	interactiveMode = false
}

// stubStdinTerminal overrides the TTY seam for the duration of a test.
func stubStdinTerminal(t *testing.T, v bool) {
	t.Helper()
	old := isStdinTerminal
	t.Cleanup(func() { isStdinTerminal = old })
	isStdinTerminal = func() bool { return v }
}

func TestRun_bareTTY_returnsErrInteractive(t *testing.T) {
	resetInteractiveMode(t)
	stubStdinTerminal(t, true)

	err := run(nil)
	if !errors.Is(err, errInteractive) {
		t.Fatalf("run(nil) on TTY = %v, want errInteractive", err)
	}
}

func TestRun_bareEmptySlice_TTY_returnsErrInteractive(t *testing.T) {
	resetInteractiveMode(t)
	stubStdinTerminal(t, true)

	if err := run([]string{}); !errors.Is(err, errInteractive) {
		t.Fatalf("run([]) on TTY = %v, want errInteractive", err)
	}
}

func TestRun_bareNonTTY_returnsMissingSubcommand(t *testing.T) {
	resetInteractiveMode(t)
	stubStdinTerminal(t, false)

	err := run(nil)
	if !errors.Is(err, errMissingSubcommand) {
		t.Fatalf("run(nil) non-TTY = %v, want errMissingSubcommand", err)
	}
}

func TestRun_bareNonTTY_interactiveFlagStillMissingSubcommand(t *testing.T) {
	resetInteractiveMode(t)
	stubStdinTerminal(t, false)

	// -i is peeled, leaving bare args: still missing-subcommand off-TTY.
	err := run([]string{"-i"})
	if !errors.Is(err, errMissingSubcommand) {
		t.Fatalf("run([-i]) non-TTY = %v, want errMissingSubcommand", err)
	}
	if !interactiveMode {
		t.Fatalf("interactiveMode = false, want true after peeling -i")
	}
}

func TestRun_bareTTY_interactiveFlagReturnsErrInteractive(t *testing.T) {
	resetInteractiveMode(t)
	stubStdinTerminal(t, true)

	err := run([]string{"--interactive"})
	if !errors.Is(err, errInteractive) {
		t.Fatalf("run([--interactive]) on TTY = %v, want errInteractive", err)
	}
}

func TestErrSentinels_zeverPrefixed(t *testing.T) {
	for name, err := range map[string]error{
		"errInteractive":       errInteractive,
		"errMissingSubcommand": errMissingSubcommand,
	} {
		if !strings.HasPrefix(err.Error(), "zever:") {
			t.Errorf("%s = %q, want zever: prefix", name, err)
		}
	}
}

func TestSubcommandHandlers_coversEverySubcommand(t *testing.T) {
	want := []string{
		"new", "compile", "doctor", "config", "routes", "check", "breaking", "fmt",
		"explain", "check-boundaries", "check:boundaries", "graph", "generate", "extract",
		"serve", "dev", "queue:work", "schedule:run", "tinker", "db",
	}
	for _, name := range want {
		t.Run(name, func(t *testing.T) {
			if _, ok := subcommandHandlers[name]; !ok {
				t.Fatalf("subcommandHandlers missing %q", name)
			}
		})
	}

	if len(subcommandHandlers) != len(want) {
		t.Fatalf("len(subcommandHandlers) = %d, want %d", len(subcommandHandlers), len(want))
	}

	// help aliases are handled inline by run, never via the table.
	for _, alias := range []string{"help", "-h", "--help"} {
		if _, ok := subcommandHandlers[alias]; ok {
			t.Fatalf("subcommandHandlers must not contain %q", alias)
		}
	}
}

func TestRun_helpVariants_returnNil(t *testing.T) {
	resetInteractiveMode(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"bare help", []string{"help"}},
		{"dash h", []string{"-h"}},
		{"dash dash help", []string{"--help"}},
		{"help unknown sub", []string{"help", "bogus-sub"}},
		{"help routes", []string{"help", "routes"}},
		{"help explain", []string{"help", "explain"}},
		{"help check-boundaries", []string{"help", "check-boundaries"}},
		{"help graph", []string{"help", "graph"}},
		{"help queue:work (no dedicated usage)", []string{"help", "queue:work"}},
		{"help schedule:run (no dedicated usage)", []string{"help", "schedule:run"}},
		{"help new", []string{"help", "new"}},
		{"help compile", []string{"help", "compile"}},
		{"help check", []string{"help", "check"}},
		{"help breaking", []string{"help", "breaking"}},
		{"help fmt", []string{"help", "fmt"}},
		{"help doctor", []string{"help", "doctor"}},
		{"help generate", []string{"help", "generate"}},
		{"help db", []string{"help", "db"}},
		{"help extract", []string{"help", "extract"}},
		{"help serve", []string{"help", "serve"}},
		{"help dev", []string{"help", "dev"}},
		{"help tinker", []string{"help", "tinker"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetInteractiveMode(t)
			if err := run(tc.args); err != nil {
				t.Fatalf("run(%v) = %v, want nil", tc.args, err)
			}
		})
	}
}

func TestRun_unknownSubcommand_error(t *testing.T) {
	resetInteractiveMode(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"gibberish", []string{"frobnicate"}},
		{"typo of compile", []string{"compil"}},
		{"typo of generate", []string{"generat"}},
		{"empty string sub", []string{""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetInteractiveMode(t)
			err := run(tc.args)
			if err == nil {
				t.Fatalf("run(%v) = nil, want unknown-subcommand error", tc.args)
			}
			if !strings.HasPrefix(err.Error(), "zever: unknown subcommand") {
				t.Fatalf("run(%v) = %q, want zever: unknown subcommand prefix", tc.args, err)
			}
			if errors.Is(err, errInteractive) || errors.Is(err, errMissingSubcommand) {
				t.Fatalf("run(%v) = %v, must not match sentinels", tc.args, err)
			}
		})
	}
}

func TestRun_unknownSubcommand_hintsSilenced(t *testing.T) {
	resetInteractiveMode(t)
	t.Setenv("ZEVER_NO_HINT", "1")

	err := run([]string{"compil"})
	if err == nil || !strings.HasPrefix(err.Error(), "zever: unknown subcommand") {
		t.Fatalf("run([compil]) = %v, want unknown-subcommand error", err)
	}
}

func TestPeelInteractive(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		wantArgs []string
		wantMode bool
	}{
		{"no flags", []string{"new", "myapp"}, []string{"new", "myapp"}, false},
		{"short global", []string{"-i", "new"}, []string{"new"}, true},
		{"long global", []string{"--interactive", "new"}, []string{"new"}, true},
		{"trailing", []string{"new", "-i"}, []string{"new"}, true},
		{"empty", nil, []string{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetInteractiveMode(t)
			t.Setenv("ZEVER_INTERACTIVE", "")

			got := peelInteractive(tc.args)
			if len(got) != len(tc.wantArgs) {
				t.Fatalf("peelInteractive(%v) = %v, want %v", tc.args, got, tc.wantArgs)
			}
			for i := range got {
				if got[i] != tc.wantArgs[i] {
					t.Fatalf("peelInteractive(%v) = %v, want %v", tc.args, got, tc.wantArgs)
				}
			}
			if interactiveMode != tc.wantMode {
				t.Fatalf("interactiveMode = %v, want %v", interactiveMode, tc.wantMode)
			}
		})
	}
}

func TestPeelInteractive_env(t *testing.T) {
	resetInteractiveMode(t)
	t.Setenv("ZEVER_INTERACTIVE", "1")

	got := peelInteractive([]string{"new"})
	if len(got) != 1 || got[0] != "new" {
		t.Fatalf("peelInteractive = %v, want [new]", got)
	}
	if !interactiveMode {
		t.Fatalf("interactiveMode = false, want true via ZEVER_INTERACTIVE=1")
	}
}

func TestEnvInteractive(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  bool
	}{
		{"one", "1", true},
		{"true", "true", true},
		{"yes", "yes", true},
		{"upper TRUE", "TRUE", true},
		{"padded yes", "  yes  ", true},
		{"zero", "0", false},
		{"empty", "", false},
		{"no", "no", false},
		{"false", "false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ZEVER_INTERACTIVE", tc.value)
			if got := envInteractive(); got != tc.want {
				t.Fatalf("envInteractive() with ZEVER_INTERACTIVE=%q = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// errDispatchProbe is returned by the stubbed handler in
// TestRun_dispatchesToHandler.
var errDispatchProbe = errors.New("zever: dispatch probe")

func TestRun_dispatchesToHandler(t *testing.T) {
	resetInteractiveMode(t)
	stubStdinTerminal(t, false)

	const name = "test-dispatch-probe"

	prev, had := subcommandHandlers[name]
	t.Cleanup(func() {
		if had {
			subcommandHandlers[name] = prev
		} else {
			delete(subcommandHandlers, name)
		}
	})

	var gotArgs []string

	subcommandHandlers[name] = func(args []string) error {
		gotArgs = append([]string(nil), args...)
		return errDispatchProbe
	}

	if err := run([]string{name, "a", "b"}); !errors.Is(err, errDispatchProbe) {
		t.Fatalf("run([probe a b]) = %v, want dispatch probe", err)
	}

	if len(gotArgs) != 2 || gotArgs[0] != "a" || gotArgs[1] != "b" {
		t.Fatalf("handler args = %v, want [a b]", gotArgs)
	}
}

func TestPrintUsage_colorBranches(t *testing.T) {
	prev := colorEnabled
	t.Cleanup(func() { colorEnabled = prev })

	colorEnabled = true

	out := captureStderr(t, func() { printUsage() })
	if !strings.Contains(out, "╭") {
		t.Fatalf("color printUsage lacks box frame:\n%s", out)
	}

	if !strings.Contains(out, "Usage:") {
		t.Fatalf("color printUsage lacks Usage:\n%s", out)
	}

	colorEnabled = false

	out = captureStderr(t, func() { printUsage() })
	if strings.Contains(out, "╭") {
		t.Fatalf("plain printUsage must not draw a box:\n%s", out)
	}

	if !strings.Contains(out, "Usage:") {
		t.Fatalf("plain printUsage lacks Usage:\n%s", out)
	}
}

// ---- main() subprocess coverage (GOCOVERDIR, no real TTY) ----

// runCoverChild runs the coverage-built binary with the given stdio wiring
// and returns its exit code plus captured stderr. Bounded by ctx.
func runCoverChild(ctx context.Context, t *testing.T, bin string, coverDir string, args []string, stdin io.Reader, stderr io.Writer, extraEnv []string) int {
	t.Helper()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+coverDir)
	cmd.Env = append(cmd.Env, extraEnv...)

	if stdin != nil {
		cmd.Stdin = stdin
	}

	if stderr != nil {
		cmd.Stderr = stderr
	}

	cmd.Stdout = io.Discard

	err := cmd.Run()
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	t.Fatalf("child run: %v", err)

	return -1
}

func TestMain_subprocessCover(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatalf("need python3 (stdlib pty driver) for the pty-backed dashboard run: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "zever")
	coverDir := filepath.Join(tmp, "coverdata")

	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatalf("mkdir coverdata: %v", err)
	}

	build := exec.CommandContext(ctx, "go", "build", "-cover", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build -cover: %v\n%s", err, out)
	}

	devnull, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer func() { _ = devnull.Close() }()

	devnullErr, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open /dev/null for stderr: %v", err)
	}
	defer func() { _ = devnullErr.Close() }()

	// 1. --help over pipes: run returns nil, main falls through (exit 0).
	var helpErr bytes.Buffer

	if code := runCoverChild(ctx, t, bin, coverDir, []string{"--help"}, nil, &helpErr, nil); code != 0 {
		t.Fatalf("--help exit = %d, want 0\nstderr:\n%s", code, helpErr.String())
	}

	if !strings.Contains(helpErr.String(), "Usage:") {
		t.Fatalf("--help stderr lacks Usage:\n%s", helpErr.String())
	}

	// 2. Bare with piped stdin (not a char device): missing-subcommand,
	// error printed without color (stderr is a pipe), exit 1.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = pr.Close() }()
	defer func() { _ = pw.Close() }()

	var bareErr bytes.Buffer

	if code := runCoverChild(ctx, t, bin, coverDir, nil, pr, &bareErr, nil); code != 1 {
		t.Fatalf("bare-pipe exit = %d, want 1\nstderr:\n%s", code, bareErr.String())
	}

	if !strings.Contains(bareErr.String(), "missing subcommand") {
		t.Fatalf("bare-pipe stderr lacks missing subcommand:\n%s", bareErr.String())
	}

	// 3. Bare with /dev/null stdin (/dev/null is a char device, so the
	// conservative isStdinTerminal reports a TTY): errInteractive, then the
	// dashboard program fails headless — wrapped zever: dashboard error,
	// printed without color, exit 1.
	var dashErr bytes.Buffer

	if code := runCoverChild(ctx, t, bin, coverDir, nil, devnull, &dashErr, nil); code != 1 {
		t.Fatalf("bare-devnull exit = %d, want 1\nstderr:\n%s", code, dashErr.String())
	}

	if !strings.Contains(dashErr.String(), "dashboard") {
		t.Fatalf("bare-devnull stderr lacks dashboard:\n%s", dashErr.String())
	}

	// 4. Same dashboard failure but with a char-device stderr and TERM set:
	// the error prints through the color branch (output discarded).
	if code := runCoverChild(ctx, t, bin, coverDir, nil, devnull, devnullErr,
		[]string{"TERM=xterm-256color", "NO_COLOR="}); code != 1 {
		t.Fatalf("bare-devnull-color exit = %d, want 1", code)
	}

	// 5. Unknown subcommand with char-device stderr: generic error through
	// the color branch, exit 1.
	if code := runCoverChild(ctx, t, bin, coverDir, []string{"frobnicate"}, pr, devnullErr,
		[]string{"TERM=xterm-256color", "NO_COLOR="}); code != 1 {
		t.Fatalf("unknown-color exit = %d, want 1", code)
	}

	// 6. Bare on a real pty (raw mode, stdlib pty driver below): genuine
	// TTY, dashboard starts, ctrl+c quits cleanly — dashboard success,
	// exit 0. Raw mode matters: a cooked pty would line-buffer the byte
	// (never delivered) or turn it into SIGINT (no clean exit, no
	// coverdata flush). Draining the master matters too: the dashboard
	// repaints continuously and would block on a full output buffer,
	// starving its input loop.
	driver := filepath.Join(tmp, "pty_quit.py")
	if writeErr := os.WriteFile(driver, []byte(ptyQuitDriver), 0o600); writeErr != nil {
		t.Fatalf("write pty driver: %v", writeErr)
	}
	// Owner-execute so the pty child can run it (WriteFile above stays 0600).
	if chmodErr := os.Chmod(driver, 0o700); chmodErr != nil {
		t.Fatalf("chmod pty driver: %v", chmodErr)
	}

	ptyCmd := exec.CommandContext(ctx, "python3", driver, bin, coverDir)

	var ptyOut bytes.Buffer
	ptyCmd.Stdout = &ptyOut
	ptyCmd.Stderr = &ptyOut

	if runErr := ptyCmd.Run(); runErr != nil {
		t.Fatalf("pty dashboard run: %v\noutput:\n%s", runErr, ptyOut.String())
	}

	if !strings.Contains(ptyOut.String(), "child-exit=0") {
		t.Fatalf("pty dashboard lacks child-exit=0:\n%s", ptyOut.String())
	}

	// Prove main() itself ran under coverage: convert the children's
	// coverdata and require func main to be fully covered (mirrors
	// tools/zever-lsp/main_test.go).
	entries, err := os.ReadDir(coverDir)
	if err != nil {
		t.Fatalf("read coverdata: %v", err)
	}

	if len(entries) == 0 {
		t.Fatalf("GOCOVERDIR %s is empty, want coverage output from main()", coverDir)
	}

	funcCov := exec.CommandContext(ctx, "go", "tool", "covdata", "func", "-i="+coverDir) //nolint:gosec // test-only: coverDir is a t.TempDir() path, never user input
	out, covErr := funcCov.CombinedOutput()

	if covErr != nil {
		t.Fatalf("go tool covdata func: %v\n%s", covErr, out)
	}

	t.Logf("subprocess coverdata:\n%s", covdataMainLines(t, out))

	mainCovered := false

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}

		if strings.Contains(fields[0], "main.go:") && fields[1] == "main" && fields[2] == "100.0%" {
			mainCovered = true
		}
	}

	if !mainCovered {
		t.Errorf("subprocess coverdata lacks 100%% func main coverage:\n%s", out)
	}
}

// ptyQuitDriver is a stdlib-only python3 program: it allocates a raw-mode
// pty (slave is the child's stdin/stdout/stderr), drains the master while
// the dashboard runs, sends one literal ctrl+c byte, and exits 0 only when
// the child exits 0. argv: <binary> <coverdir>.
const ptyQuitDriver = `import fcntl, os, pty, select, struct, subprocess, sys, termios, time, tty
bincov, coverdir = sys.argv[1], sys.argv[2]
m, s = pty.openpty()
tty.setraw(s)
fcntl.ioctl(s, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 80, 0, 0))
env = dict(os.environ, GOCOVERDIR=coverdir, TERM="xterm-256color", NO_COLOR="")
p = subprocess.Popen([bincov], stdin=s, stdout=s, stderr=s, env=env, close_fds=True)
os.close(s)
os.set_blocking(m, False)
t0 = time.time()
sent = False
while True:
    if p.poll() is not None:
        break
    if time.time() - t0 > 25:
        print("TIMEOUT waiting for dashboard exit")
        p.kill()
        sys.exit(99)
    r, _, _ = select.select([m], [], [], 0.3)
    if r:
        try:
            if not os.read(m, 65536):
                break
        except OSError:
            break
    if not sent and time.time() - t0 > 2.0:
        os.write(m, b"\x03")
        sent = True
rc = p.wait(timeout=10)
print(f"child-exit={rc}")
sys.exit(0 if rc == 0 else 42)
`

// covdataMainLines extracts the main.go func lines for the test log.
func covdataMainLines(t *testing.T, out []byte) string {
	t.Helper()

	var b strings.Builder

	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "main.go:") {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return b.String()
}
