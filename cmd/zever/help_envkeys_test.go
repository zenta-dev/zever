package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

// TestCLINeverNamesALegacyConfigFile pins the clean break: no source file of
// the CLI package names a pre-v0.8.0 config file -- not the help text, not a
// comment, not a golden file. The environment is the only configuration
// surface, so the name has to be gone from every word the CLI prints and
// from every file it writes.
func TestCLINeverNamesALegacyConfigFile(t *testing.T) {
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

	for _, path := range files {
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
			want: []string{"<BATTERY>_ADAPTER", "CACHE_ADAPTER=redis", ".env at the project root"},
		},
		{
			name: "generate worker",
			body: workerUsageBody,
			want: []string{"QUEUE_ADAPTER=redis", ".env at the project root", "ZEVER_WORKER_ENTRY"},
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
