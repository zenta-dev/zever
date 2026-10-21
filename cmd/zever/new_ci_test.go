package main

import (
	"os"
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

// TestRecipeCIWorkflow proves the `zever add ci` recipe template
// regenerates derived code and gates drift with `zever compile --check`, so
// every project gets the eject/drift gate the day it opts in.
func TestRecipeCIWorkflow(t *testing.T) {
	t.Parallel()

	content := renderCIGitHubWorkflow("1.24", "v0.6.1", "zenorm,proto,atlas")

	for _, fragment := range []string{
		"zever compile --check",
		"zever compile --backend=",
		"go build ./...",
		"go test ./...",
		"install.sh",
		"--version v0.6.1",
		`>> "$GITHUB_PATH"`,
		"runs-on: ubuntu-24.04",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("scaffolded ci.yml lacks %q:\n%s", fragment, content)
		}
	}

	if strings.Contains(content, "go install") {
		t.Errorf("scaffolded ci.yml must not use go install (uninstallable before the first release that ships a replace-free cmd/zever):\n%s", content)
	}

	if strings.Contains(content, "ubuntu-latest") {
		t.Errorf("scaffolded ci.yml must pin the runner image (ubuntu-latest migrates to Ubuntu 26):\n%s", content)
	}
}

// TestRenderCIWorkflowLocalCheckoutPinsLatest proves scaffolds bound to a
// local framework checkout (pseudo-version, uninstallable) let the installer
// pick its default release instead of emitting an uninstallable version.
func TestRenderCIWorkflowLocalCheckoutPinsLatest(t *testing.T) {
	t.Parallel()

	content := renderCIGitHubWorkflow(defaultGoVersion, pseudoVersionZero, "zenorm,proto,atlas")

	if !strings.Contains(content, "install.sh") || strings.Contains(content, "--version") {
		t.Errorf("local-checkout ci.yml should run install.sh without --version:\n%s", content)
	}

	if strings.Contains(content, pseudoVersionZero) {
		t.Errorf("local-checkout ci.yml leaks pseudo-version:\n%s", content)
	}
}

// TestRecipeFilesCI proves the ci recipe writes exactly the workflow file
// with valid content.
func TestRecipeFilesCI(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/shop\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	files, err := recipeFiles("ci")
	if err != nil {
		t.Fatalf("recipeFiles(ci): %v", err)
	}

	if len(files) != 1 || files[0].rel != filepath.Join(".github", "workflows", "ci.yml") {
		t.Fatalf("ci recipe files = %v, want the workflow only", files)
	}

	if !strings.Contains(files[0].content, "zever compile --check") {
		t.Fatalf("ci recipe content lacks the drift gate:\n%s", files[0].content)
	}
}

// TestRecipeFilesDocker proves the docker recipe writes the Dockerfile and
// .dockerignore with the reference content, and never copies config.
func TestRecipeFilesDocker(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	files, err := recipeFiles("docker")
	if err != nil {
		t.Fatalf("recipeFiles(docker): %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("docker recipe files = %v, want Dockerfile + .dockerignore", files)
	}

	for _, f := range files {
		if f.rel == "Dockerfile" {
			for _, fragment := range []string{
				"FROM golang:", "distroless", "USER nonroot", "HEALTHCHECK", "/healthz",
			} {
				if !strings.Contains(f.content, fragment) {
					t.Errorf("Dockerfile lacks %q", fragment)
				}
			}

			// No config file and no secret may be baked into the image:
			// config comes from the environment at run time.
			for _, banned := range []string{".yaml", ".env", "SECRET", "ENV DB_"} {
				if strings.Contains(f.content, banned) {
					t.Errorf("Dockerfile must not contain %q", banned)
				}
			}
		}
	}
}
