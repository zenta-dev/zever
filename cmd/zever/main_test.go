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

func TestRun_bareNil_returnsMissingSubcommand(t *testing.T) {
	resetInteractiveMode(t)

	err := run(nil)
	if !errors.Is(err, errMissingSubcommand) {
		t.Fatalf("run(nil) = %v, want errMissingSubcommand", err)
	}
}

func TestRun_bareEmptySlice_returnsMissingSubcommand(t *testing.T) {
	resetInteractiveMode(t)

	if err := run([]string{}); !errors.Is(err, errMissingSubcommand) {
		t.Fatalf("run([]) = %v, want errMissingSubcommand", err)
	}
}

func TestRun_bareInteractiveFlag_returnsMissingSubcommand(t *testing.T) {
	resetInteractiveMode(t)

	// -i is peeled, leaving bare args: still missing-subcommand.
	err := run([]string{"-i"})
	if !errors.Is(err, errMissingSubcommand) {
		t.Fatalf("run([-i]) = %v, want errMissingSubcommand", err)
	}
	if !interactiveMode {
		t.Fatalf("interactiveMode = false, want true after peeling -i")
	}
}

func TestRun_bareLongInteractiveFlag_returnsMissingSubcommand(t *testing.T) {
	resetInteractiveMode(t)

	err := run([]string{"--interactive"})
	if !errors.Is(err, errMissingSubcommand) {
		t.Fatalf("run([--interactive]) = %v, want errMissingSubcommand", err)
	}
}

func TestErrSentinels_zeverPrefixed(t *testing.T) {
	for name, err := range map[string]error{
		"errMissingSubcommand": errMissingSubcommand,
	} {
		if !strings.HasPrefix(err.Error(), "zever:") {
			t.Errorf("%s = %q, want zever: prefix", name, err)
		}
	}
}

func TestSubcommandHandlers_coversEverySubcommand(t *testing.T) {
	want := []string{
		"new", "add", "compile", "doctor", "config", "routes", "check", "breaking", "fmt",
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

func TestRun_versionFlag_printsVersionToStdout(t *testing.T) {
	resetInteractiveMode(t)
	for _, args := range [][]string{{"--version"}, {"-V"}} {
		resetInteractiveMode(t)
		out := captureStdout(t, func() {
			if err := run(args); err != nil {
				t.Fatalf("run(%v) = %v, want nil", args, err)
			}
		})
		if !strings.Contains(out, "zever v"+cliVersion) {
			t.Fatalf("run(%v) stdout = %q, want %q", args, out, "zever v"+cliVersion)
		}
	}

	resetInteractiveMode(t)
	errOut := captureStderr(t, func() {
		if err := run([]string{"--version"}); err != nil {
			t.Fatalf("run([--version]) = %v, want nil", err)
		}
	})
	if errOut != "" {
		t.Fatalf("run([--version]) stderr = %q, want empty (version goes to stdout)", errOut)
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
			if errors.Is(err, errMissingSubcommand) {
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

	return runCoverChildOut(ctx, t, bin, coverDir, args, stdin, nil, stderr, extraEnv)
}

// runCoverChildOut is runCoverChild with an optional stdout capture. A nil
// stdout discards it. Cobra prints --help to stdout (uniform with kubectl/gh
// style CLIs); errors and bare-invocation usage stay on stderr.
func runCoverChildOut(ctx context.Context, t *testing.T, bin string, coverDir string, args []string, stdin io.Reader, stdout, stderr io.Writer, extraEnv []string) int {
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

	if stdout != nil {
		cmd.Stdout = stdout
	} else {
		cmd.Stdout = io.Discard
	}

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
	// The cover build below compiles the whole cmd/zever import closure
	// (~1500 packages) with coverage instrumentation, which takes ~60s
	// from a cold build cache on a fast machine and longer on loaded CI
	// runners compiling sibling packages concurrently -- well past a tight
	// budget. The timeout only bounds a hung build, not test speed.
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
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

	devnullErr, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open /dev/null for stderr: %v", err)
	}
	defer func() { _ = devnullErr.Close() }()

	// 1. --help over pipes: Cobra prints help to stdout, exit 0.
	var helpOut, helpErr bytes.Buffer

	if code := runCoverChildOut(ctx, t, bin, coverDir, []string{"--help"}, nil, &helpOut, &helpErr, nil); code != 0 {
		t.Fatalf("--help exit = %d, want 0\nstdout:\n%s", code, helpOut.String())
	}

	if !strings.Contains(helpOut.String(), "Usage:") {
		t.Fatalf("--help stdout lacks Usage:\n%s", helpOut.String())
	}

	// 2. Bare with piped stdin: missing-subcommand, error printed without
	// color (stderr is a pipe), exit 1.
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

	// 3. Unknown subcommand with char-device stderr: generic error through
	// the color branch, exit 1.
	if code := runCoverChild(ctx, t, bin, coverDir, []string{"frobnicate"}, pr, devnullErr,
		[]string{"TERM=xterm-256color", "NO_COLOR="}); code != 1 {
		t.Fatalf("unknown-color exit = %d, want 1", code)
	}
}

func TestUseCobra_routing(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"nil", nil, false},
		{"empty", []string{}, false},
		{"bare global flag", []string{"-i"}, false},
		{"version long", []string{"--version"}, false},
		{"version short", []string{"-V"}, false},
		{"help alias", []string{"help"}, false},
		{"help ported sub", []string{"help", "new"}, false},
		{"unknown sub", []string{"frobnicate"}, false},
		{"inspect compile ported", []string{"compile"}, true},
		{"inspect routes ported", []string{"routes"}, true},
		{"inspect alias ported", []string{"check:boundaries"}, true},
		{"completion ported", []string{"completion", "bash"}, true},
		{"docs ported", []string{"docs", "--dir", "man"}, true},
		{"unknown flag first routes to Cobra for exit-2 mapping", []string{"--bogus", "new"}, true},
		{"new", []string{"new", "myapp"}, true},
		{"global short before", []string{"-i", "new"}, true},
		{"global long before", []string{"--interactive", "generate"}, true},
		{"global quiet before", []string{"--quiet", "dev"}, true},
		{"generate", []string{"generate", "entity"}, true},
		{"extract", []string{"extract", "shop"}, true},
		{"serve", []string{"serve"}, true},
		{"dev", []string{"dev"}, true},
		{"queue work", []string{"queue:work"}, true},
		{"schedule run", []string{"schedule:run"}, true},
		{"tinker", []string{"tinker"}, true},
		{"bare db", []string{"db"}, true},
		{"db help flag", []string{"db", "-h"}, true},
		{"db migrate", []string{"db", "migrate", "schema/app.zen"}, true},
		{"db rollback", []string{"db", "rollback"}, true},
		{"db seed", []string{"db", "seed"}, true},
		{"db unknown stays legacy", []string{"db", "frobnicate"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := useCobra(tc.args); got != tc.want {
				t.Fatalf("useCobra(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestErrFlagUsage_zeverPrefixedAndIs(t *testing.T) {
	if !strings.HasPrefix(errFlagUsage.Error(), "zever:") {
		t.Fatalf("errFlagUsage = %q, want zever: prefix", errFlagUsage)
	}

	r := newRootCmd()
	r.SetArgs([]string{"--bogus-flag"})

	err := r.Execute()
	if !errors.Is(err, errFlagUsage) {
		t.Fatalf("Execute(--bogus-flag) = %v, want errFlagUsage", err)
	}
}
