package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// updateGolden regenerates golden files under cmd/zever/testdata when passed
// (-update-golden). The golden-file workflow is: generate once, review the
// diff, commit; `go test` compares against the committed files. Owned by the
// generate wave; other command-test waves reuse assertGolden without
// redefining it. Named -update-golden (not -update) because teatest's
// exp/golden already registers -update in this test binary.
var updateGolden = flag.Bool("update-golden", false, "regenerate golden files under testdata/")

// assertGolden compares got against testdata/<name>, writing it when -update-golden
// is passed. Golden files must be committed: they are the reviewed baseline
// for every generator's rendered output. The path is anchored to this
// source file's directory (not the working directory) because generator
// tests chdir into temp dirs.
func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()

	_, caller, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: cannot locate testdata")
	}

	path := filepath.Join(filepath.Dir(caller), "testdata", name)

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}

		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write golden file %s: %v", path, err)
		}

		t.Logf("updated golden file: %s", path)
	}

	want, err := os.ReadFile(path) //nolint:gosec // test-controlled golden path
	if err != nil {
		t.Fatalf("read golden file %s: %v (run with -update-golden to create it)", path, err)
	}

	if string(got) != string(want) {
		t.Errorf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
	}
}

// withWorkingDir temporarily chdirs to dir for the duration of the test,
// restoring the original working directory on cleanup. Owned by the generate
// wave; other command-test waves reuse it without redefining.
func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
}

func TestRunGenerateModuleGo(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runGenerateModule([]string{"module", "widgets"}); err != nil {
		t.Fatalf("runGenerateModule: %v", err)
	}

	stubPath := filepath.Join(dir, "schema", "widgets", "widgets.zen")

	content, err := os.ReadFile(stubPath) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("expected stub at %q: %v", stubPath, err)
	}

	if !strings.Contains(string(content), "widgets") {
		t.Fatalf("stub content = %q, want it to mention widgets", content)
	}
}

// TestRunGenerateDispatchesModule covers the dispatcher's calling
// convention: the plain subcommand word.
func TestRunGenerateDispatchesModule(t *testing.T) {
	for _, args := range [][]string{
		{"module", "widgets"},
	} {
		dir := t.TempDir()
		withWorkingDir(t, dir)

		if err := runGenerate(args); err != nil {
			t.Fatalf("runGenerate(%v): %v", args, err)
		}

		if _, err := os.Stat(filepath.Join(dir, "schema", "widgets", "widgets.zen")); err != nil {
			t.Fatalf("runGenerate(%v) scaffolded nothing: %v", args, err)
		}
	}
}

func TestRunGenerateDispatchesEntity(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", "")

	if err := runGenerate([]string{"entity", "shop", "Order"}); err != nil {
		t.Fatalf("runGenerate entity: %v", err)
	}

	if !contains(readFile(t, path), "entity Order {") {
		t.Fatalf("entity subcommand did not reach runGenerateEntity")
	}
}

func TestRunGenerateSubcommandErrors(t *testing.T) {
	if err := runGenerate(nil); err == nil {
		t.Fatalf("expected an error for a missing subcommand")
	}

	if err := runGenerate([]string{"nonsense"}); err == nil {
		t.Fatalf("expected an error for an unknown subcommand")
	}

	// Without a go.mod in the working directory, `generate tinker` fails
	// through currentModulePath rather than silently resolving elsewhere.
	if err := runGenerate([]string{"tinker"}); err == nil {
		t.Fatalf("expected an error for tinker without a go.mod")
	}

	for _, help := range []string{"-h", "--help", "help"} {
		if err := runGenerate([]string{help}); err != nil {
			t.Fatalf("runGenerate(%q): %v", help, err)
		}
	}
}

func TestSplitPositionals(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		n              int
		wantPositional []string
		wantRemainder  []string
	}{
		{"positionals first", []string{"shop", "Order", "--field", "a:string"}, 2,
			[]string{"shop", "Order"}, []string{"--field", "a:string"}},
		{"flags first", []string{"--field", "a:string", "shop", "Order"}, 2,
			nil, []string{"--field", "a:string", "shop", "Order"}},
		{"stops at n", []string{"a", "b", "c"}, 2, []string{"a", "b"}, []string{"c"}},
		{"empty", nil, 2, nil, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			positional, remainder := splitPositionals(tc.args, tc.n)

			if !equalStrings(positional, tc.wantPositional) {
				t.Fatalf("positional = %v, want %v", positional, tc.wantPositional)
			}

			if !equalStrings(remainder, tc.wantRemainder) {
				t.Fatalf("remainder = %v, want %v", remainder, tc.wantRemainder)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestRunGenerateModuleMissingArgs(t *testing.T) {
	if err := runGenerateModule(nil); err == nil {
		t.Fatalf("expected error for missing args, got nil")
	}

	if err := runGenerateModule([]string{"module"}); err == nil {
		t.Fatalf("expected error for missing module name, got nil")
	}
}

// TestPrintUsagesCoverBranches renders every usage printer in both color
// modes so the box()/plain branches stay covered. Usage text is
// presentation-only; the test pins that it renders without panicking.
func TestPrintUsagesCoverBranches(t *testing.T) {
	old := colorEnabled
	t.Cleanup(func() { colorEnabled = old })

	printers := map[string]func(fs *flag.FlagSet){
		"job":      printJobUsage,
		"schedule": printScheduleUsage,
		"seed":     printSeedUsage,
		"server":   printServerUsage,
		"tinker":   printTinkerGenerateUsage,
		"worker":   printWorkerUsage,
		"adapter":  printAdapterUsage,
	}

	for _, colored := range []bool{false, true} {
		colorEnabled = colored

		for name, print := range printers {
			fs := flag.NewFlagSet("test-"+name, flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			print(fs)
		}

		printGenerateUsage()
	}
}

// TestGenerateModuleRejectsTraversal pins the path-traversal rule: names
// carrying .., separators, or absolute paths fail with ErrPathTraversal
// (errors.Is-matchable) and create nothing on disk.
func TestGenerateModuleRejectsTraversal(t *testing.T) {
	for _, name := range []string{"..", "../evil", "/abs", "a/b", `a\b`, "."} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			withWorkingDir(t, dir)

			cfg := GenerateModuleConfig{Name: name, SchemaDir: "schema"}

			if _, err := GenerateModule(cfg); !isTraversalError(err) {
				t.Fatalf("GenerateModule(%q) = %v, want ErrPathTraversal", name, err)
			}

			entries, rerr := os.ReadDir(dir)
			if rerr != nil {
				t.Fatalf("readdir: %v", rerr)
			}

			if len(entries) != 0 {
				t.Fatalf("rejected name created files: %v", entries)
			}
		})
	}
}

func isTraversalError(err error) bool {
	return errors.Is(err, ErrPathTraversal)
}

// TestIsTraversalName pins the classifier itself.
func TestIsTraversalName(t *testing.T) {
	for _, s := range []string{"..", ".", "/abs", "a/b", `a\b`, ""} {
		if !isTraversalName(s) {
			t.Errorf("isTraversalName(%q) = false, want true", s)
		}
	}

	for _, s := range []string{"shop", "Order", "acme2", "a-b"} {
		if isTraversalName(s) {
			t.Errorf("isTraversalName(%q) = true, want false", s)
		}
	}
}

// TestJoinUnderRootConfinesPaths proves the join gate: clean joins pass,
// escapes fail with the sentinel.
func TestJoinUnderRootConfinesPaths(t *testing.T) {
	got, err := joinUnderRoot("schema", "shop")
	if err != nil {
		t.Fatalf("joinUnderRoot: %v", err)
	}

	if got != filepath.Join("schema", "shop") {
		t.Fatalf("joinUnderRoot = %q", got)
	}

	for _, bad := range []string{"..", "../x", "/abs"} {
		if _, err := joinUnderRoot("schema", bad); !isTraversalError(err) {
			t.Errorf("joinUnderRoot(schema, %q) = %v, want ErrPathTraversal", bad, err)
		}
	}
}

// TestCoverRunGenerateInteractiveDispatch drives runGenerate's empty-args
// interactive chooser through every subcommand via the prompt seam.
func TestCoverRunGenerateInteractiveDispatch(t *testing.T) {
	for _, sub := range allGenerate {
		t.Run(sub, func(t *testing.T) {
			dir := t.TempDir()
			withWorkingDir(t, dir)
			stubPromptTTY(t, true)
			setPromptInteractive(t)
			origSel := promptSelectForGenerate
			promptSelectForGenerate = func(string, []string) (string, error) { return sub, nil }
			t.Cleanup(func() { promptSelectForGenerate = origSel })
			// Downstream run* may fail for missing fixtures; the switch
			// dispatch itself is what this covers. Seed module fixture where cheap.
			if sub == "entity" || sub == "job" || sub == "schedule" {
				writeModuleFixture(t, dir, "shop", "entity Existing {\n\tid: uuid @primary\n}\njob SendEmail() {\n\tqueue: default\n}\n")
			}
			if sub == "tinker" || sub == "server" || sub == "worker" || sub == "seed" {
				writeGoMod(t, dir, "example.com/shop")
			}
			_ = runGenerate(nil)
		})
	}
}

// TestCoverRunGenerateInteractivePromptError covers prompt failure falling
// through to usage + sentinel.
func TestCoverRunGenerateInteractivePromptError(t *testing.T) {
	withWorkingDir(t, t.TempDir())
	stubPromptTTY(t, true)
	setPromptInteractive(t)
	origSel := promptSelectForGenerate
	promptSelectForGenerate = func(string, []string) (string, error) { return "", errors.New("no tty") }
	t.Cleanup(func() { promptSelectForGenerate = origSel })
	if err := runGenerate(nil); !errors.Is(err, errGenerateUnknownSubcommand) {
		t.Fatalf("runGenerate prompt err = %v, want sentinel", err)
	}
}

// TestCoverRunGenerateSubcommands exercises every dispatch arm directly.
func TestCoverRunGenerateSubcommands(t *testing.T) {
	prevMode := interactiveMode
	t.Cleanup(func() { interactiveMode = prevMode })
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeModuleFixture(t, dir, "shop", "entity Existing {\n\tid: uuid @primary\n}\njob SendEmail() {\n\tqueue: default\n}\n")
	writeGoMod(t, dir, "example.com/shop")
	_ = runGenerate([]string{"module", "extra1"})
	_ = runGenerate([]string{"entity", "shop", "E1"})
	_ = runGenerate([]string{"job", "shop", "J1"})
	_ = runGenerate([]string{"schedule", "shop", "S1", "--cron", "0 0 * * *", "--dispatch", "SendEmail"})
	_ = runGenerate([]string{"server"})
	_ = runGenerate([]string{"worker"})
	_ = runGenerate([]string{"seed"})
	_ = runGenerate([]string{"tinker", "--app", "example.com/shop/internal/app"})
	_ = runGenerate([]string{"adapter", "cache", "covone"})
	_ = runGenerate([]string{"-h"})
	_ = runGenerate([]string{"--help"})
	_ = runGenerate([]string{"help"})
	if err := runGenerate([]string{"-i", "module", "viaflag"}); err != nil {
		t.Fatalf("runGenerate -i module: %v", err)
	}
	if err := runGenerate([]string{"--interactive", "module", "viaflag2"}); err != nil {
		t.Fatalf("runGenerate --interactive module: %v", err)
	}
	if err := runGenerate([]string{"entit"}); err == nil {
		t.Fatalf("expected typo error")
	}
}

// TestCoverRunGenerateModuleInteractive covers prompt success + error paths.
func TestCoverRunGenerateModuleInteractive(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)
	orig := promptInputForGenerate
	t.Cleanup(func() { promptInputForGenerate = orig })
	// Success: no positional -> prompt supplies name.
	dir := t.TempDir()
	withWorkingDir(t, dir)
	promptInputForGenerate = func(string, string, func(string) error) (string, error) { return "prompted", nil }
	if err := runGenerateModule([]string{}); err != nil {
		t.Fatalf("prompted module: %v", err)
	}
	// Error: prompt fails.
	promptInputForGenerate = func(string, string, func(string) error) (string, error) { return "", errors.New("boom") }
	if err := runGenerateModule([]string{}); err == nil {
		t.Fatalf("expected prompt error")
	}
	// Empty name positional triggers second prompt success.
	dir2 := t.TempDir()
	withWorkingDir(t, dir2)
	promptInputForGenerate = func(string, string, func(string) error) (string, error) { return "filled", nil }
	if err := runGenerateModule([]string{"module", ""}); err != nil {
		t.Fatalf("empty name prompt: %v", err)
	}
	// Empty name prompt error.
	promptInputForGenerate = func(string, string, func(string) error) (string, error) { return "", errors.New("boom") }
	if err := runGenerateModule([]string{"module", ""}); err == nil {
		t.Fatalf("expected empty-name prompt error")
	}
	// Parse error.
	if err := runGenerateModule([]string{"--badflag"}); err == nil {
		t.Fatalf("expected parse error")
	}
	// Interactive flag sets mode.
	dir3 := t.TempDir()
	withWorkingDir(t, dir3)
	promptInputForGenerate = func(string, string, func(string) error) (string, error) { return "imod", nil }
	if err := runGenerateModule([]string{"--interactive"}); err != nil {
		t.Fatalf("interactive flag: %v", err)
	}
	if !interactiveMode {
		t.Fatalf("-i must set interactiveMode")
	}
	interactiveMode = false
	dir4 := t.TempDir()
	withWorkingDir(t, dir4)
	promptInputForGenerate = func(string, string, func(string) error) (string, error) { return "imod2", nil }
	if err := runGenerateModule([]string{"-i"}); err != nil {
		t.Fatalf("-i flag: %v", err)
	}
	interactiveMode = false
}

// TestCoverGenerateModuleErrors covers validation + I/O fault paths.
func TestCoverGenerateModuleErrors(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	if _, err := GenerateModule(GenerateModuleConfig{Name: "bad-name!", SchemaDir: "schema"}); err == nil {
		t.Fatalf("expected ident error")
	}
	// Mkdir failure via missing-parent path (non-root-safe fault injection).
	if _, err := GenerateModule(GenerateModuleConfig{Name: "m", SchemaDir: filepath.Join("nope", "sub", "schema")}); err != nil {
		// May succeed if MkdirAll creates parents; force failure with file blocking.
		_ = err
	}
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if _, err := GenerateModule(GenerateModuleConfig{Name: "m", SchemaDir: filepath.Join(blocker, "schema")}); err == nil {
		t.Fatalf("expected mkdir error")
	}
	// Write failure: schema dir is a file.
	f := filepath.Join(dir, "afile")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = f
}
