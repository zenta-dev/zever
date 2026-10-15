package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderNewGoModPinsEveryFrameworkModule guards the published-mode
// scaffold against mixed intra-repo versions: released go.mod files can
// require older sibling modules than the scaffold's framework version, and
// Go's minimal version selection would then pick the older sibling (for
// example shared/registry without symbols the newer core modules call). So
// every framework module the checkout ships under core/, shared/ and
// adapters/ must be required at exactly the framework version.
func TestRenderNewGoModPinsEveryFrameworkModule(t *testing.T) {
	t.Parallel()

	const version = "v9.9.9"

	data, err := renderNewGoMod("zever new", NewConfig{
		ModulePath:       "example.com/acme",
		GoVersion:        "1.24",
		FrameworkVersion: version,
	})
	if err != nil {
		t.Fatalf("renderNewGoMod: %v", err)
	}

	pinned := map[string]string{}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "require "))
		if len(fields) >= 2 && strings.HasPrefix(fields[0], frameworkModulePath+"/") {
			pinned[fields[0]] = fields[1]
		}
	}

	root := repoRootAbs(t)

	var dirs []string

	for _, top := range []string{"core", "shared"} {
		entries, rerr := os.ReadDir(filepath.Join(root, top))
		if rerr != nil {
			t.Fatalf("read %s: %v", top, rerr)
		}

		for _, e := range entries {
			if _, serr := os.Stat(filepath.Join(root, top, e.Name(), "go.mod")); serr == nil {
				dirs = append(dirs, top+"/"+e.Name())
			}
		}
	}

	dirs = append(dirs, nestedModuleDirs...)

	for _, dir := range dirs {
		path := frameworkModulePath + "/" + dir
		if got := pinned[path]; got != version {
			t.Errorf("go.mod pins %s at %q, want %q", path, got, version)
		}
	}
}
