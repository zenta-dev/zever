package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// scaffoldTestConfig builds a minimal NewConfig writing into a fresh temp
// dir, the shared starting point for scaffold-output assertions.
func scaffoldTestConfig(t *testing.T, version string) NewConfig {
	t.Helper()

	return NewConfig{
		Name:             "acme",
		OutDir:           filepath.Join(t.TempDir(), "acme"),
		ModulePath:       "acme",
		GoVersion:        defaultGoVersion,
		FrameworkVersion: version,
		Batteries:        batteriesFor(nil, ""),
	}
}

// TestRunNewWritesCIWorkflow proves `zever new` scaffolds a CI workflow that
// regenerates derived code and gates drift with `zever compile --check`, so
// every new project gets the eject/drift gate from day one.
func TestRunNewWritesCIWorkflow(t *testing.T) {
	t.Parallel()

	cfg := scaffoldTestConfig(t, "v0.6.1")

	written, err := writeNewProject("zever new", cfg)
	if err != nil {
		t.Fatalf("writeNewProject: %v", err)
	}

	rel := filepath.Join(".github", "workflows", "ci.yml")
	content := readFile(t, filepath.Join(cfg.OutDir, rel))

	for _, fragment := range []string{
		"zever compile --check",
		"zever compile --backend=",
		"go build ./...",
		"go test ./...",
		"install.sh",
		"--version v0.6.1",
		`>> "$GITHUB_PATH"`,
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("scaffolded ci.yml lacks %q:\n%s", fragment, content)
		}
	}

	if strings.Contains(content, "go install") {
		t.Errorf("scaffolded ci.yml must not use go install (uninstallable before the first release that ships replace-free cmd/zever):\n%s", content)
	}

	found := false

	for _, path := range written {
		if path == filepath.Join(cfg.OutDir, rel) {
			found = true
		}
	}

	if !found {
		t.Errorf("ci.yml not in written list: %v", written)
	}
}

// TestRenderCIWorkflowLocalCheckoutPinsLatest proves scaffolds bound to a
// local framework checkout (pseudo-version, uninstallable) let the installer
// pick its default release instead of emitting an uninstallable version.
func TestRenderCIWorkflowLocalCheckoutPinsLatest(t *testing.T) {
	t.Parallel()

	cfg := scaffoldTestConfig(t, pseudoVersionZero)

	if _, err := writeNewProject("zever new", cfg); err != nil {
		t.Fatalf("writeNewProject: %v", err)
	}

	content := readFile(t, filepath.Join(cfg.OutDir, ".github", "workflows", "ci.yml"))

	if !strings.Contains(content, "install.sh") || strings.Contains(content, "--version") {
		t.Errorf("local-checkout ci.yml should run install.sh without --version:\n%s", content)
	}

	if strings.Contains(content, pseudoVersionZero) {
		t.Errorf("local-checkout ci.yml leaks pseudo-version:\n%s", content)
	}
}
