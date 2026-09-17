package main

import (
	"errors"
	"reflect"
	"testing"
)

// launchCall records one stubbed execLaunch invocation.
type launchCall struct {
	entry string
	args  []string
}

// stubLaunch replaces execLaunch for the duration of a test and returns a
// pointer to the recorded calls.
func stubLaunch(t *testing.T, err error) *[]launchCall {
	t.Helper()

	calls := &[]launchCall{}
	orig := execLaunch

	execLaunch = func(entryDir string, passthroughArgs []string) error {
		*calls = append(*calls, launchCall{entry: entryDir, args: passthroughArgs})
		return err
	}

	t.Cleanup(func() { execLaunch = orig })

	return calls
}

// assertOneLaunch asserts exactly one launch happened with the given entry
// and passthrough arguments.
func assertOneLaunch(t *testing.T, calls *[]launchCall, entry string, args []string) {
	t.Helper()

	if len(*calls) != 1 {
		t.Fatalf("execLaunch called %d time(s), want 1: %+v", len(*calls), *calls)
	}

	got := (*calls)[0]
	if got.entry != entry {
		t.Fatalf("launched %q, want %q", got.entry, entry)
	}

	if !reflect.DeepEqual(got.args, args) {
		t.Fatalf("passthrough args = %#v, want %#v", got.args, args)
	}
}

func TestRunServeUsesDefaultServerEntry(t *testing.T) {
	t.Chdir(t.TempDir())

	calls := stubLaunch(t, nil)

	if err := runServe([]string{"-addr", ":9090"}); err != nil {
		t.Fatalf("runServe: %v", err)
	}

	assertOneLaunch(t, calls, defaultServerEntry, []string{"-addr", ":9090"})
}

func TestRunServeUsesConfiguredServerEntry(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  server_entry: cmd/api\n")
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	if err := runServe(nil); err != nil {
		t.Fatalf("runServe: %v", err)
	}

	assertOneLaunch(t, calls, "cmd/api", nil)
}

func TestRunServePropagatesLaunchError(t *testing.T) {
	t.Chdir(t.TempDir())

	want := errors.New("boom")
	_ = stubLaunch(t, want)

	if err := runServe(nil); !errors.Is(err, want) {
		t.Fatalf("runServe error = %v, want %v", err, want)
	}
}

func TestRunServeHelpDoesNotLaunch(t *testing.T) {
	t.Chdir(t.TempDir())

	calls := stubLaunch(t, nil)

	if err := runServe([]string{"-h"}); err != nil {
		t.Fatalf("runServe -h: %v", err)
	}

	if len(*calls) != 0 {
		t.Fatalf("-h launched a child: %+v", *calls)
	}
}

// TestHasHelpFlagScansAllArgs verifies hasHelpFlag scans all args for a help
// flag rather than only the first.
func TestHasHelpFlagScansAllArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "trailing -h", args: []string{"-addr", ":80", "-h"}, want: true},
		{name: "leading --help", args: []string{"--help"}, want: true},
		{name: "bare help", args: []string{"help"}, want: true},
		{name: "nil", args: nil, want: false},
		{name: "no help", args: []string{"-addr", ":80"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := hasHelpFlag(tt.args); got != tt.want {
				t.Fatalf("hasHelpFlag(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// TestResolveServeConfig pins the XConfig refactor: project layout plus CLI
// args resolve to a plain config with no side effects.
func TestResolveServeConfig(t *testing.T) {
	t.Parallel()

	project := ProjectConfig{ServerEntry: "cmd/api"}.withDefaults()
	cfg := resolveServeConfig(project, []string{"-addr", ":9090"})

	if cfg.Entry != "cmd/api" {
		t.Fatalf("Entry = %q, want %q", cfg.Entry, "cmd/api")
	}

	if !reflect.DeepEqual(cfg.Args, []string{"-addr", ":9090"}) {
		t.Fatalf("Args = %#v, want %#v", cfg.Args, []string{"-addr", ":9090"})
	}
}

// TestRunServeBadConfig proves a broken project file surfaces as an error
// instead of launching with half-decoded layout.
func TestRunServeBadConfig(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  server_entry: [unclosed\n")
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	if err := runServe(nil); err == nil {
		t.Fatal("expected a config error, got nil")
	}

	if len(*calls) != 0 {
		t.Fatalf("bad config launched a child: %+v", *calls)
	}
}
