package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
)

// stubPromptTTY overrides the isStdinTerminal seam for the test duration and
// resets interactiveMode plus ZEVER_INTERACTIVE so each test controls the
// full gate.
func stubPromptTTY(t *testing.T, tty bool) {
	t.Helper()

	prevSeam := isStdinTerminal
	isStdinTerminal = func() bool { return tty }
	t.Cleanup(func() { isStdinTerminal = prevSeam })

	prevMode := interactiveMode
	interactiveMode = false
	t.Cleanup(func() { interactiveMode = prevMode })

	t.Setenv("ZEVER_INTERACTIVE", "")
}

// setPromptInteractive enables interactiveMode for the test duration.
func setPromptInteractive(t *testing.T) {
	t.Helper()

	prev := interactiveMode
	interactiveMode = true
	t.Cleanup(func() { interactiveMode = prev })
}

// captureStderr redirects os.Stderr to a pipe while fn runs and returns what
// was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	prev := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stderr = w
	defer func() { os.Stderr = prev }()

	fn()

	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close pipe writer: %v", closeErr)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	return string(out)
}

// captureStdout redirects os.Stdout to a pipe while fn runs and returns what
// was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stdout = w
	defer func() { os.Stdout = prev }()

	fn()

	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close pipe writer: %v", closeErr)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	return string(out)
}

// writeZenFixture writes content at rel under dir, creating parents.
func writeZenFixture(t *testing.T, dir, rel, content string) {
	t.Helper()

	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestIsInteractiveTerminal_flagAndTTY_required(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		env         string
		tty         bool
		want        bool
	}{
		{"neither_requested_tty", false, "", true, false},
		{"neither_requested_no_tty", false, "", false, false},
		{"flag_without_tty", true, "", false, false},
		{"flag_with_tty", true, "", true, true},
		{"env_without_tty", false, "1", false, false},
		{"env_with_tty", false, "yes", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubPromptTTY(t, tt.tty)
			if tt.interactive {
				setPromptInteractive(t)
			}

			if tt.env != "" {
				t.Setenv("ZEVER_INTERACTIVE", tt.env)
			}

			if got := isInteractiveTerminal(); got != tt.want {
				t.Errorf("isInteractiveTerminal = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequireInteractive_allowsTTY(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	if err := requireInteractive(); err != nil {
		t.Fatalf("requireInteractive: %v", err)
	}
}

func TestRequireInteractive_flagWithoutTTY_mentionsTTY(t *testing.T) {
	stubPromptTTY(t, false)
	setPromptInteractive(t)

	err := requireInteractive()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "TTY") {
		t.Errorf("error = %q, want mention of TTY", err.Error())
	}
}

func TestRequireInteractive_unrequested_namesInteractiveFlag(t *testing.T) {
	stubPromptTTY(t, false)

	err := requireInteractive()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "--interactive") {
		t.Errorf("error = %q, want mention of --interactive", err.Error())
	}
}

func TestHasInteractiveFlag_table(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		want bool
	}{
		{"empty", nil, "", false},
		{"short", []string{"-i"}, "", true},
		{"long", []string{"new", "--interactive"}, "", true},
		{"unrelated", []string{"compile", "a.zen"}, "", false},
		{"env", nil, "true", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ZEVER_INTERACTIVE", tt.env)
			if got := hasInteractiveFlag(tt.args); got != tt.want {
				t.Errorf("hasInteractiveFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestPromptHelpers_nonInteractive_error(t *testing.T) {
	stubPromptTTY(t, false)

	if _, err := promptInput("Name", "", nil); err == nil {
		t.Error("promptInput: expected error, got nil")
	}

	if _, err := promptSelect("Pick", []string{"a"}); err == nil {
		t.Error("promptSelect: expected error, got nil")
	}

	if _, err := promptMultiSelect("Pick", []string{"a"}); err == nil {
		t.Error("promptMultiSelect: expected error, got nil")
	}

	if _, err := promptConfirm("Sure?", "Yes"); err == nil {
		t.Error("promptConfirm: expected error, got nil")
	}
}

func TestPromptSelect_emptyOptions(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	if _, err := promptSelect("Pick", nil); err == nil {
		t.Fatal("expected error for empty options, got nil")
	}
}

func TestPromptMultiSelect_emptyOptions(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	if _, err := promptMultiSelect("Pick", nil); err == nil {
		t.Fatal("expected error for empty options, got nil")
	}
}

func TestDiscoverZenFiles_recursive(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeZenFixture(t, dir, "schema/app.zen", "app {}\n")
	writeZenFixture(t, dir, "schema/shop/order.zen", "order {}\n")
	writeZenFixture(t, dir, "schema/README.md", "not a schema\n")

	files := discoverZenFiles()
	if len(files) != 2 {
		t.Fatalf("discoverZenFiles = %v, want 2 files", files)
	}

	for _, f := range files {
		if !strings.HasSuffix(f, ".zen") {
			t.Errorf("discoverZenFiles returned non-.zen file %q", f)
		}
	}
}

func TestDiscoverZenFiles_missingDir_empty(t *testing.T) {
	t.Chdir(t.TempDir())

	if files := discoverZenFiles(); len(files) != 0 {
		t.Errorf("discoverZenFiles = %v, want empty", files)
	}
}

func TestResolveInputFiles_explicitPassthrough(t *testing.T) {
	explicit := []string{"a.zen", "b.zen"}

	got, err := resolveInputFiles(explicit)
	if err != nil {
		t.Fatalf("resolveInputFiles: %v", err)
	}

	if len(got) != len(explicit) || got[0] != explicit[0] || got[1] != explicit[1] {
		t.Errorf("resolveInputFiles = %v, want %v", got, explicit)
	}
}

func TestResolveInputFiles_discoversWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeZenFixture(t, dir, "schema/app.zen", "app {}\n")

	got, err := resolveInputFiles(nil)
	if err != nil {
		t.Fatalf("resolveInputFiles: %v", err)
	}

	if len(got) != 1 || !strings.HasSuffix(got[0], filepath.Join("schema", "app.zen")) {
		t.Errorf("resolveInputFiles = %v, want [schema/app.zen]", got)
	}
}

func TestResolveInputFiles_none_yieldsSentinel(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, err := resolveInputFiles(nil); !errors.Is(err, errNoInputFiles) {
		t.Errorf("error = %v, want errNoInputFiles", err)
	}
}

func TestDiscoverModules_listsDirsWithZen(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeZenFixture(t, dir, "schema/shop/order.zen", "order {}\n")
	writeZenFixture(t, dir, "schema/billing/invoice.zen", "invoice {}\n")
	if err := os.MkdirAll(filepath.Join(dir, "schema", "empty"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	writeZenFixture(t, dir, "schema/top.zen", "top {}\n")

	got := discoverModules()
	if len(got) != 2 {
		t.Fatalf("discoverModules = %v, want [billing shop]", got)
	}

	if got[0] != "billing" || got[1] != "shop" {
		t.Errorf("discoverModules = %v, want sorted [billing shop]", got)
	}
}

func TestDiscoverModules_missingDir_nil(t *testing.T) {
	t.Chdir(t.TempDir())

	if got := discoverModules(); got != nil {
		t.Errorf("discoverModules = %v, want nil", got)
	}
}

func TestReportAutoDiscovery_silentWhenDisabled(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "1")

	out := captureStderr(t, func() {
		reportAutoDiscovery([]string{"schema/app.zen"})
	})
	if out != "" {
		t.Errorf("reportAutoDiscovery wrote %q with hints disabled", out)
	}

	out = captureStderr(t, func() {
		reportAutoDiscovery(nil)
	})
	if out != "" {
		t.Errorf("reportAutoDiscovery wrote %q for empty input", out)
	}
}

func TestReportAutoDiscovery_reportsCount(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "")
	t.Chdir(t.TempDir())

	out := captureStderr(t, func() {
		reportAutoDiscovery([]string{"a.zen", "b.zen"})
	})

	if !strings.Contains(out, "auto-discovered 2 schema file(s)") {
		t.Errorf("reportAutoDiscovery wrote %q, want auto-discovery hint", out)
	}
}

// ---- headless huh driving (no real TTY, bounded time) ----

// stubRunForm replaces the runForm seam for a test. Callers drive the real
// huh form headlessly (driveFormTeatest) or return a synthetic error for
// cancel-path coverage.
func stubRunForm(t *testing.T, fn func(*huh.Form) error) {
	t.Helper()

	prev := runForm
	t.Cleanup(func() { runForm = prev })
	runForm = fn
}

// huhTeaAdapter bridges huh v2 forms to tea.Model for teatest: huh's
// Update returns (huh.Model, tea.Cmd) and View returns string, so *huh.Form
// does not satisfy tea.Model directly. The adapter holds the same *huh.Form
// pointer, so values bound via huh.Value pointers stay visible to callers.
type huhTeaAdapter struct{ form *huh.Form }

func (a huhTeaAdapter) Init() tea.Cmd { return a.form.Init() }

func (a huhTeaAdapter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := a.form.Update(msg)
	if nf, ok := m.(*huh.Form); ok {
		a.form = nf
	}

	return a, cmd
}

func (a huhTeaAdapter) View() tea.View { return tea.NewView(a.form.View()) }

// driveFormTeatest runs f on teatest's virtual terminal: optionally types
// text, feeds keys in order, quits, and waits bounded. The form mutates in
// place, so values bound via huh.Value pointers are visible to the caller.
func driveFormTeatest(t *testing.T, f *huh.Form, typeText string, keys ...tea.Msg) {
	t.Helper()

	tm := teatest.NewTestModel(t, huhTeaAdapter{form: f}, teatest.WithInitialTermSize(80, 24))
	if typeText != "" {
		tm.Type(typeText)
	}

	for _, k := range keys {
		tm.Send(k)
	}

	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}

	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
}

// promptKeys builds KeyPressMsgs: promptRune for printable input,
// promptSpecial for codes like tea.KeyEnter/tea.KeyDown.
func promptRune(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func promptSpecial(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

func TestRunForm_default_headlessErrors(t *testing.T) {
	// Proves the default seam body (form.Run) executes: headless it must
	// fail fast (no TTY) rather than hang. Keeps the seam itself covered.
	f := huh.NewForm(huh.NewGroup(huh.NewInput().Title("x")))
	if err := runForm(f); err == nil {
		t.Fatal("runForm default headless = nil, want TTY error")
	}
}

func TestPromptInput_success_teatest(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	// Non-nil validator exercises the field.Validate wiring; it accepts.
	stubRunForm(t, func(f *huh.Form) error {
		driveFormTeatest(t, f, "  spaced  ", promptSpecial(tea.KeyEnter))
		return nil
	})

	got, err := promptInput("Name", "name", func(string) error { return nil })
	if err != nil {
		t.Fatalf("promptInput: %v", err)
	}

	if got != "spaced" {
		t.Fatalf("promptInput = %q, want trimmed %q", got, "spaced")
	}
}

func TestPromptInput_validateDirect(t *testing.T) {
	// huh Validate funcs are unit-testable without a terminal: call them
	// directly for accept/reject, then prove the wiring attaches them.
	wantErr := errors.New("no blanks")
	validate := func(s string) error {
		if strings.TrimSpace(s) == "" {
			return wantErr
		}

		return nil
	}

	if err := validate("ok"); err != nil {
		t.Fatalf("validate(ok) = %v, want nil", err)
	}

	if err := validate("   "); !errors.Is(err, wantErr) {
		t.Fatalf("validate(blank) = %v, want sentinel", err)
	}

	stubPromptTTY(t, true)
	setPromptInteractive(t)
	stubRunForm(t, func(f *huh.Form) error {
		driveFormTeatest(t, f, "wired", promptSpecial(tea.KeyEnter))
		return nil
	})

	if _, err := promptInput("Name", "", validate); err != nil {
		t.Fatalf("promptInput with validator: %v", err)
	}
}

func TestPromptInput_cancel_wrapsError(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	stubRunForm(t, func(*huh.Form) error { return errors.New("abort") })

	_, err := promptInput("Name", "", nil)
	if err == nil {
		t.Fatal("promptInput cancel = nil, want error")
	}

	if !strings.Contains(err.Error(), `prompt "Name"`) {
		t.Fatalf("promptInput cancel = %q, want prompt-title wrap", err)
	}
}

func TestPromptSelect_success_teatest(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	t.Run("enter picks focused first", func(t *testing.T) {
		stubRunForm(t, func(f *huh.Form) error {
			driveFormTeatest(t, f, "", promptSpecial(tea.KeyEnter))
			return nil
		})

		got, err := promptSelect("Pick", []string{"alpha", "beta"})
		if err != nil {
			t.Fatalf("promptSelect: %v", err)
		}

		if got != "alpha" {
			t.Fatalf("promptSelect = %q, want %q", got, "alpha")
		}
	})

	t.Run("down enter picks second", func(t *testing.T) {
		stubRunForm(t, func(f *huh.Form) error {
			driveFormTeatest(t, f, "", promptSpecial(tea.KeyDown), promptSpecial(tea.KeyEnter))
			return nil
		})

		got, err := promptSelect("Pick", []string{"alpha", "beta"})
		if err != nil {
			t.Fatalf("promptSelect: %v", err)
		}

		if got != "beta" {
			t.Fatalf("promptSelect = %q, want %q", got, "beta")
		}
	})
}

func TestPromptSelect_cancel_wrapsError(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	stubRunForm(t, func(*huh.Form) error { return errors.New("abort") })

	_, err := promptSelect("Pick", []string{"a"})
	if err == nil {
		t.Fatal("promptSelect cancel = nil, want error")
	}

	if !strings.Contains(err.Error(), `prompt "Pick"`) {
		t.Fatalf("promptSelect cancel = %q, want prompt-title wrap", err)
	}
}

func TestPromptConfirm_success_teatest(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	t.Run("yes", func(t *testing.T) {
		stubRunForm(t, func(f *huh.Form) error {
			driveFormTeatest(t, f, "", promptRune('y'), promptSpecial(tea.KeyEnter))
			return nil
		})

		got, err := promptConfirm("Sure?", "Yes")
		if err != nil {
			t.Fatalf("promptConfirm: %v", err)
		}

		if !got {
			t.Fatal("promptConfirm(y) = false, want true")
		}
	})

	t.Run("no", func(t *testing.T) {
		stubRunForm(t, func(f *huh.Form) error {
			driveFormTeatest(t, f, "", promptRune('n'), promptSpecial(tea.KeyEnter))
			return nil
		})

		got, err := promptConfirm("Sure?", "Yes")
		if err != nil {
			t.Fatalf("promptConfirm: %v", err)
		}

		if got {
			t.Fatal("promptConfirm(n) = true, want false")
		}
	})
}

func TestPromptConfirm_cancel_wrapsError(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	stubRunForm(t, func(*huh.Form) error { return errors.New("abort") })

	_, err := promptConfirm("Sure?", "Yes")
	if err == nil {
		t.Fatal("promptConfirm cancel = nil, want error")
	}

	if !strings.Contains(err.Error(), `prompt "Sure?"`) {
		t.Fatalf("promptConfirm cancel = %q, want prompt-title wrap", err)
	}
}

func TestPromptMultiSelect_success_teatest(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	stubRunForm(t, func(f *huh.Form) error {
		driveFormTeatest(t, f, "", promptSpecial(tea.KeySpace), promptSpecial(tea.KeyEnter))
		return nil
	})

	got, err := promptMultiSelect("Pick", []string{"a", "b"})
	if err != nil {
		t.Fatalf("promptMultiSelect: %v", err)
	}

	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("promptMultiSelect = %v, want [a]", got)
	}
}

func TestPromptMultiSelect_cancel_wrapsError(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	stubRunForm(t, func(*huh.Form) error { return errors.New("abort") })

	_, err := promptMultiSelect("Pick", []string{"a"})
	if err == nil {
		t.Fatal("promptMultiSelect cancel = nil, want error")
	}

	if !strings.Contains(err.Error(), `prompt "Pick"`) {
		t.Fatalf("promptMultiSelect cancel = %q, want prompt-title wrap", err)
	}
}

func TestPromptMultiSelectDefault_preselected_teatest(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	// Enter with no toggles: the pre-checked floor value survives, proving
	// the picker's "floor pre-selected, user can deselect" wiring.
	stubRunForm(t, func(f *huh.Form) error {
		driveFormTeatest(t, f, "", promptSpecial(tea.KeyEnter))
		return nil
	})

	got, err := promptMultiSelectDefault("Pick", []string{"a", "b"}, []string{"b"})
	if err != nil {
		t.Fatalf("promptMultiSelectDefault: %v", err)
	}

	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("promptMultiSelectDefault = %v, want [b]", got)
	}
}

func TestPromptMultiSelectDefault_emptyOptions(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	if _, err := promptMultiSelectDefault("Pick", nil, []string{"a"}); err == nil {
		t.Fatal("expected error for empty options, got nil")
	}
}

func TestPromptMultiSelectDefault_nonInteractive_error(t *testing.T) {
	stubPromptTTY(t, false)

	if _, err := promptMultiSelectDefault("Pick", []string{"a"}, nil); err == nil {
		t.Error("promptMultiSelectDefault: expected error, got nil")
	}
}
