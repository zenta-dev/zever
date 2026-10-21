package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// exampleAdapterDir maps a "<battery>/<adapter>" pair as selected in an
// example's .env.example to the adapters/<battery>/<dir> package that
// implements it. Most pairs map one-to-one; the entries here cover adapters
// whose package directory name differs from the adapter selector.
var exampleAdapterDir = map[string]string{
	"password/argon2id": "adapters/password/argon2",
	"search/db":         "adapters/search/db",
	"vectorstore/db":    "adapters/vectorstore/db",
	"workflow/db":       "adapters/workflow/db",
	"resilience/memory": "adapters/resilience/inproc",
}

// TestExampleAppsRegisterConfiguredAdapters is the guard that every example
// app wires the adapter it selects. Adapters never self-register, so an app
// that selects an adapter it never imports fails at startup with "unknown
// adapter"; this test catches that omission at build time instead.
//
// Config is env-only since v0.8.0, so the selection lives in each example's
// .env.example (<SERVICE>_ADAPTER=<adapter>) and the wiring in internal/app:
// app.go carries the Register() calls, wiring.go the typed container.Provide
// entries. A battery the example provides itself (a stub store instead of a
// shipped adapter) has no adapter package to match, so it is skipped.
func TestExampleAppsRegisterConfiguredAdapters(t *testing.T) {
	t.Parallel()

	root := repoRootAbs(t)

	envs, err := filepath.Glob(filepath.Join(root, "examples", "*", ".env.example"))
	if err != nil {
		t.Fatalf("glob example env templates: %v", err)
	}

	if len(envs) == 0 {
		t.Fatal("no examples/*/.env.example found")
	}

	for _, envPath := range envs {
		exampleDir := filepath.Dir(envPath)
		name := filepath.Base(exampleDir)

		raw, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatalf("read %s: %v", envPath, err)
		}

		imports := exampleAdapterImports(t, exampleDir)
		if imports == "" {
			t.Logf("skip %s: no internal/app package", name)

			continue
		}

		for _, key := range selectedAdapters(string(raw)) {
			battery, _, _ := strings.Cut(key, "/")

			used := adapterPackagesFor(imports, battery)
			if len(used) == 0 {
				// The example provides this battery itself, so no shipped
				// adapter package is expected.
				continue
			}

			dir, ok := exampleAdapterDir[key]
			if !ok {
				dir = "adapters/" + key
			}

			want := `"github.com/zenta-dev/zever/` + dir + `"`

			if !strings.Contains(imports, want) {
				t.Errorf("%s: selected %s but no %s import (has %s)", name, key, dir, strings.Join(used, ", "))
			}
		}
	}
}

// selectedAdapters returns the "<battery>/<adapter>" pairs an example
// selects through <SERVICE>_ADAPTER or <SERVICE>__<NAME>__ADAPTER.
func selectedAdapters(envExample string) []string {
	var out []string

	for _, line := range strings.Split(envExample, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.ToUpper(strings.TrimSpace(key))
		if !strings.HasSuffix(key, "_ADAPTER") {
			continue
		}

		head, _, _ := strings.Cut(key, "__")
		head = strings.TrimSuffix(head, "_ADAPTER")
		if head == "" {
			continue
		}

		out = append(out, strings.ToLower(head)+"/"+strings.TrimSpace(value))
	}

	return out
}

// exampleAdapterImports concatenates the non-test Go sources of an
// example's internal/app package, so both the Register() calls in app.go and
// the typed container.Provide entries in wiring.go count. An empty result
// means the example has no internal/app package at all.
func exampleAdapterImports(t *testing.T, exampleDir string) string {
	t.Helper()

	appDir := filepath.Join(exampleDir, "internal", "app")

	entries, err := os.ReadDir(appDir)
	if err != nil {
		return ""
	}

	var sb strings.Builder

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(appDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}

		sb.Write(raw)
	}

	return sb.String()
}

// adapterImportPattern matches an adapter import line and captures the
// battery and the adapter directory.
var adapterImportPattern = regexp.MustCompile(`zenta-dev/zever/adapters/([a-z0-9]+)/([a-z0-9]+)`)

// adapterPackagesFor lists the adapter packages the example imports for one
// battery, for example adapters/db/sqlite.
func adapterPackagesFor(imports, battery string) []string {
	var out []string

	seen := map[string]bool{}

	for _, m := range adapterImportPattern.FindAllStringSubmatch(imports, -1) {
		if m[1] != battery {
			continue
		}

		pkg := "adapters/" + m[1] + "/" + m[2]
		if !seen[pkg] {
			seen[pkg] = true

			out = append(out, pkg)
		}
	}

	return out
}
