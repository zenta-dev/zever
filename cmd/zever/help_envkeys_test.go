package main

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// legacyConfigNames are the pre-v0.8.0 project config file names. The CLI
// must never name one: service configuration has been env-only since
// v0.8.0, so a name in the help (or in a file the CLI scaffolds) would send
// the user to a file the framework silently ignores. The names are assembled
// from parts so this guard does not reintroduce the strings it forbids.
var legacyConfigNames = []string{
	"zever" + ".yaml",
	"zever" + ".yml",
	"zever" + ".json",
	"zever" + ".toml",
}

// legacyProjectKeys are the pre-v0.8.0 project-layout stanza keys. The
// layout moved to the environment in v0.8.0, so naming one of them sends
// the user to a knob that does nothing. The names are assembled from parts
// so this guard does not reintroduce the strings it forbids.
var legacyProjectKeys = []string{
	"project" + "." + "server_entry",
	"project" + "." + "worker_entry",
	"project" + "." + "seed_entry",
	"project" + "." + "tinker_entry",
	"project" + "." + "generated_dir",
	"project" + "." + "schema_dir",
}

// TestCLINeverNamesALegacyProjectKey pins the clean break for the layout
// knobs: no source file of the CLI package names a project-layout stanza
// key -- not the help text, not an error message, not a comment. The layout
// is env-only since v0.8.0, so every knob has to be named by its env key.
func TestCLINeverNamesALegacyProjectKey(t *testing.T) {
	for _, path := range cliPackageFiles(t) {
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Fatalf("read %s: %v", path, rerr)
		}

		for _, key := range legacyProjectKeys {
			if strings.Contains(string(body), key) {
				t.Errorf("%s names %s", path, key)
			}
		}
	}
}

// TestCLINeverNamesALegacyConfigFile pins the clean break: no source file of
// the CLI package names a pre-v0.8.0 config file -- not the help text, not a
// comment, not a golden file. The environment is the only configuration
// surface, so the name has to be gone from every word the CLI prints and
// from every file it writes.
func TestCLINeverNamesALegacyConfigFile(t *testing.T) {
	for _, path := range cliPackageFiles(t) {
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Fatalf("read %s: %v", path, rerr)
		}

		lower := strings.ToLower(string(body))

		for _, name := range legacyConfigNames {
			if strings.Contains(lower, name) {
				t.Errorf("%s names %s", path, name)
			}
		}
	}
}

// TestUsageNamesTheEnvKey pins the wording the help has to carry: the env
// key that selects the adapter and the file it belongs in. A user who
// follows the help must be able to act on it without guessing.
func TestUsageNamesTheEnvKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "generate adapter",
			body: adapterUsageBody,
			want: []string{"container.Provide", "internal/app/wiring.go", "no adapter env key"},
		},
		{
			name: "generate worker",
			body: workerUsageBody,
			want: []string{"container.Provide", "internal/app/wiring.go", "ZEVER_WORKER_ENTRY"},
		},
		{
			name: "generate server",
			body: serverUsageBody,
			want: []string{"ZEVER_SERVER_ENTRY"},
		},
		{
			name: "generate seed",
			body: seedUsageBody,
			want: []string{"ZEVER_SEED_ENTRY"},
		},
		{
			name: "generate tinker",
			body: tinkerGenerateUsageBody,
			want: []string{"ZEVER_TINKER_ENTRY"},
		},
		{
			name: "new",
			body: newUsage,
			want: []string{".env"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, want := range tc.want {
				if !strings.Contains(tc.body, want) {
					t.Errorf("%s help must name %q:\n%s", tc.name, want, tc.body)
				}
			}
		})
	}
}

// TestRuntimeHelpNamesTheEnvKey pins the command help of the launcher
// commands: the knob that moves the entrypoint is named by its env key, so
// a user who follows the help lands on a knob that exists.
func TestRuntimeHelpNamesTheEnvKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		long string
		want string
	}{
		{name: "serve", long: newServeCmd().Long, want: "ZEVER_SERVER_ENTRY"},
		{name: "queue:work", long: newQueueWorkCmd().Long, want: "ZEVER_WORKER_ENTRY"},
		{name: "worker", long: newWorkerCmd().Long, want: "ZEVER_WORKER_ENTRY"},
		{name: "db seed", long: dbSeedCommand(t).Long, want: "ZEVER_SEED_ENTRY"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(tc.long, tc.want) {
				t.Errorf("%s help must name %q:\n%s", tc.name, tc.want, tc.long)
			}
		})
	}
}

// TestLauncherHelpNamesTheEnvKey pins the launcher usage the runtime
// commands print for -h: it names the env key that moves the entrypoint.
func TestLauncherHelpNamesTheEnvKey(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "zever serve", run: func() error { return runServe([]string{"-h"}) }, want: "ZEVER_SERVER_ENTRY"},
		{name: "zever worker", run: func() error { return runQueueWork([]string{"-h"}) }, want: "ZEVER_WORKER_ENTRY"},
		{name: "zever db seed", run: func() error { return runDBSeed([]string{"-h"}) }, want: "ZEVER_SEED_ENTRY"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := captureFlagOutput(t, tc.run)

			if !strings.Contains(out, tc.want) {
				t.Errorf("%s help must name %q:\n%s", tc.name, tc.want, out)
			}
		})
	}
}

// dbSeedCommand returns the `zever db seed` cobra command, so the tests can
// pin its help text without running it.
func dbSeedCommand(t *testing.T) *cobra.Command {
	t.Helper()

	for _, cmd := range newDBCmd().Commands() {
		if cmd.Name() == "seed" {
			return cmd
		}
	}

	t.Fatal("zever db: no seed subcommand")

	return nil
}

// captureFlagOutput runs fn with the stdlib flag package's output
// redirected to a buffer: the launcher usage writes there, not to the cobra
// command's writer.
func captureFlagOutput(t *testing.T, fn func() error) string {
	t.Helper()

	var buf bytes.Buffer

	flag.CommandLine.SetOutput(&buf)
	t.Cleanup(func() { flag.CommandLine.SetOutput(nil) })

	if err := fn(); err != nil {
		t.Fatalf("help: %v", err)
	}

	return buf.String()
}

// cliPackageFiles returns the source files of this package (the .go files
// and the testdata golden files), so the guards above scan the sources the
// CLI actually ships.
func cliPackageFiles(t *testing.T) []string {
	t.Helper()

	root := packageDir(t)

	var files []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}

		if d.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".go" && !strings.Contains(path, string(filepath.Separator)+"testdata"+string(filepath.Separator)) {
			return nil
		}

		files = append(files, path)

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	return files
}

// packageDir returns the directory of this package, so the guard above
// scans the sources it actually ships.
func packageDir(t *testing.T) string {
	t.Helper()

	_, caller, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: cannot locate the package directory")
	}

	return filepath.Dir(caller)
}
