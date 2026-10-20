package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFixtureModule writes a go.mod for one module of a synthetic
// framework checkout and returns its directory.
func writeFixtureModule(t *testing.T, root, dir, modulePath, body string) string {
	t.Helper()

	full := filepath.Join(root, filepath.FromSlash(dir))

	if err := os.MkdirAll(full, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}

	content := "module " + modulePath + "\n\ngo 1.27.2\n"
	if body != "" {
		content += "\n" + body
	}

	if err := os.WriteFile(filepath.Join(full, "go.mod"), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s/go.mod: %v", full, err)
	}

	return full
}

// TestScanFrameworkModulesSkipsForeignAndFixtureModules pins the scan's
// filters: only modules of the framework itself are collected, so the
// replace set never points at another org's module or at a test fixture.
func TestScanFrameworkModulesSkipsForeignAndFixtureModules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFixtureModule(t, root, "core/cache", frameworkModulePath+"/core/cache", "")
	writeFixtureModule(t, root, "shared/codec", frameworkModulePath+"/shared/codec", "")
	writeFixtureModule(t, root, "examples/external-sms", "github.com/example/zever-sms", "")
	writeFixtureModule(t, root, "container/testdata/compilefail/sealed", frameworkModulePath+"/container/testdata/compilefail/sealed", "")

	mods, err := scanFrameworkModules(root)
	if err != nil {
		t.Fatalf("scanFrameworkModules: %v", err)
	}

	want := map[string]string{
		frameworkModulePath + "/core/cache":   "core/cache",
		frameworkModulePath + "/shared/codec": "shared/codec",
	}

	if len(mods) != len(want) {
		t.Fatalf("scan returned %d modules (%v), want %d", len(mods), mods, len(want))
	}

	for path, dir := range want {
		if mods[path] != dir {
			t.Errorf("scan[%q] = %q, want %q", path, mods[path], dir)
		}
	}
}

// TestFrameworkRuntimeModulesFollowsTheModuleGraph proves the runtime set
// is the checkout's module graph: the batteries, the adapters and the
// shared helpers are roots, and every module they require inside the
// repository joins them.
func TestFrameworkRuntimeModulesFollowsTheModuleGraph(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFixtureModule(t, root, "core/cache", frameworkModulePath+"/core/cache",
		"require github.com/zenta-dev/zever/shared/codec v0.1.0\n")
	writeFixtureModule(t, root, "adapters/cache/memory", frameworkModulePath+"/adapters/cache/memory",
		"require (\n\tgithub.com/zenta-dev/zever/core/cache v0.1.0\n\tgithub.com/zenta-dev/zever/shared/codec v0.1.0\n)\n")
	writeFixtureModule(t, root, "shared/codec", frameworkModulePath+"/shared/codec", "")
	// A module no runtime root requires stays out of the set.
	writeFixtureModule(t, root, "extra/orphan", frameworkModulePath+"/extra/orphan", "")

	got, err := frameworkRuntimeModules(root, nil)
	if err != nil {
		t.Fatalf("frameworkRuntimeModules: %v", err)
	}

	want := []string{
		frameworkModulePath + "/adapters/cache/memory",
		frameworkModulePath + "/core/cache",
		frameworkModulePath + "/shared/codec",
	}

	if len(got) != len(want) {
		t.Fatalf("runtime modules = %v, want %v", got, want)
	}

	for i, path := range want {
		if got[i] != path {
			t.Errorf("runtime modules[%d] = %q, want %q", i, got[i], path)
		}
	}
}

// TestFrameworkRuntimeModulesIncludesGivenRoots proves a module the
// scaffold requires directly is part of the graph even when no runtime root
// pulls it in (the container and the serve modules).
func TestFrameworkRuntimeModulesIncludesGivenRoots(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFixtureModule(t, root, "core/cache", frameworkModulePath+"/core/cache", "")
	writeFixtureModule(t, root, "container", frameworkModulePath+"/container",
		"require github.com/zenta-dev/zever/core/cache v0.1.0\n")

	got, err := frameworkRuntimeModules(root, []string{frameworkModulePath + "/container"})
	if err != nil {
		t.Fatalf("frameworkRuntimeModules: %v", err)
	}

	want := []string{
		frameworkModulePath + "/container",
		frameworkModulePath + "/core/cache",
	}

	if len(got) != len(want) {
		t.Fatalf("runtime modules = %v, want %v", got, want)
	}

	for i, path := range want {
		if got[i] != path {
			t.Errorf("runtime modules[%d] = %q, want %q", i, got[i], path)
		}
	}
}

// TestFrameworkModuleRequiresReadsBothForms pins the require parser: the
// single-line and the block form, and the ignore of everything that is not
// an intra-repo module.
func TestFrameworkModuleRequiresReadsBothForms(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	full := writeFixtureModule(t, root, "core/cache", frameworkModulePath+"/core/cache", "")

	body := "require github.com/zenta-dev/zever/shared/codec v0.1.0\n" +
		"require (\n" +
		"\tgithub.com/zenta-dev/zever/core/log v0.1.0 // indirect\n" +
		"\tgithub.com/example/other v1.0.0\n" +
		")\n" +
		"replace github.com/zenta-dev/zever/core/log => ../log\n"

	if err := os.WriteFile(filepath.Join(full, "go.mod"), []byte("module "+frameworkModulePath+"/core/cache\n\ngo 1.27.2\n\n"+body), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	got, err := frameworkModuleRequires(filepath.Join(full, "go.mod"))
	if err != nil {
		t.Fatalf("frameworkModuleRequires: %v", err)
	}

	want := []string{
		frameworkModulePath + "/shared/codec",
		frameworkModulePath + "/core/log",
	}

	if len(got) != len(want) {
		t.Fatalf("requires = %v, want %v", got, want)
	}

	for i, path := range want {
		if got[i] != path {
			t.Errorf("requires[%d] = %q, want %q", i, got[i], path)
		}
	}
}
