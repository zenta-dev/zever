package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunNewEnvOnlyContract locks in the first-run contract: a project from
// `zever new <name>` is configured from .env alone. It scaffolds into a temp
// dir through the same runNew seam the other new tests use and asserts, on
// the files on disk, that .env and .env.example exist, that no retired
// project config file exists anywhere in the scaffolded tree, that the env
// files carry the keys the framework derives (adapter selectors and dev
// secrets from the scaffolder's own derivation, never a hardcoded guess),
// and that the README documents .env as the configuration source.
//
// What is already covered elsewhere, and deliberately not repeated here:
// TestRunNewWritesEnvFiles pins the secret shapes in .env,
// TestScaffoldedEnvKeysResolve and friends pin that every rendered key
// resolves through config.ApplyEnv, TestRenderNewReadmeTellsTheTruth pins
// the README quickstart, and TestCLINeverNamesALegacyConfigFile pins that no
// CLI source names a retired file. This test pins the on-disk artifact of
// the default first-run path, so the scaffolder cannot drift back to a file
// without a CI job noticing.
func TestRunNewEnvOnlyContract(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"envonlyapp", "--no-tidy"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	out := filepath.Join(workDir, "envonlyapp")

	for _, rel := range []string{".env", ".env.example"} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	// The expected selections come from the same derivation writeNewProject
	// renders from, so the test tracks the scaffolder instead of guessing.
	selections := NewConfig{Batteries: batteriesFor(nil, "")}.batterySelections()

	assertNoRetiredConfigFiles(t, out)

	example := readFile(t, filepath.Join(out, ".env.example"))

	for _, s := range selections {
		want := strings.ToUpper(s.Battery) + "_ADAPTER=" + s.Adapter
		if !strings.Contains(example, want) {
			t.Errorf(".env.example lacks wired selector %q", want)
		}
	}

	dev := readFile(t, filepath.Join(out, ".env"))
	env, _ := envFileKeys(t, dev)

	wantKeys := map[string]bool{}
	for _, k := range devSecretKeys(selections) {
		wantKeys[k] = true
	}

	for _, s := range selections {
		for _, k := range sortedEnvDefaultKeys(scaffoldEnvDefaults(s.Battery, "")) {
			wantKeys[k] = true
		}
	}

	for k := range wantKeys {
		if _, ok := env[k]; !ok {
			t.Errorf(".env lacks derived key %q", k)
		}
	}

	for k := range env {
		if !wantKeys[k] {
			t.Errorf(".env carries unexpected key %q", k)
		}
	}

	readme := readFile(t, filepath.Join(out, "README.md"))

	for _, want := range []string{"env-only", ".env"} {
		if !strings.Contains(readme, want) {
			t.Errorf("README must document %q as the configuration source", want)
		}
	}

	for _, s := range selections {
		want := strings.ToUpper(s.Battery) + "_ADAPTER=" + s.Adapter
		if !strings.Contains(readme, want) {
			t.Errorf("README lacks wired selector %q", want)
		}
	}
}

// assertNoRetiredConfigFiles fails when any file anywhere under dir carries
// a retired project config file name. The names come from the shared guard
// list, assembled from parts so no test reintroduces the strings it forbids;
// the scan is recursive because the root-only extension check elsewhere
// would miss a file nested deeper in the tree.
func assertNoRetiredConfigFiles(t *testing.T, dir string) {
	t.Helper()

	retired := map[string]bool{}
	for _, name := range legacyConfigNames {
		retired[strings.ToLower(name)] = true
	}

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}

		if d.IsDir() {
			return nil
		}

		if retired[strings.ToLower(d.Name())] {
			rel, _ := filepath.Rel(dir, p)
			t.Errorf("scaffolded tree must not contain %q: configuration is env-only", rel)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk scaffold: %v", err)
	}
}
