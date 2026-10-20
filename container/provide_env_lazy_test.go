package container

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
)

// captureStderr returns everything fn wrote to os.Stderr. It mirrors the
// helper in pools_info_test.go: the pipe is drained after fn returns, so
// the test must not run in parallel with other stderr writers.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stderr

	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stderr = wr

	fn()

	os.Stderr = old
	_ = wr.Close()

	b, _ := io.ReadAll(rd)
	_ = rd.Close()

	return string(b)
}

// TestNewWithoutProvidersSkipsEnvironment proves the zero-provider path
// never reads the process environment: the container does not read the
// working directory, so the legacy-config warning stays with the config
// layer (config.FromEnv), which prints it when an app resolves its own
// environment.
func TestNewWithoutProvidersSkipsEnvironment(t *testing.T) {
	// No t.Parallel: the test replaces os.Stderr and changes the working
	// directory.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zever.yaml"), []byte("db:\n  adapter: sqlite\n"), 0o600); err != nil {
		t.Fatalf("write zever.yaml: %v", err)
	}

	t.Chdir(dir)

	got := captureStderr(t, func() {
		c := New(nil)
		mustCloseContainer(t, c)
	})

	if got != "" {
		t.Fatalf("zero-provider container wrote to stderr: %q, want none", got)
	}
}

// TestNewWithProvidersReadsEnvironmentAtBuild proves the provider path
// still reads the process environment, and that it reads it when the first
// provider is built: the legacy-config warning appears there, not at New.
func TestNewWithProvidersReadsEnvironmentAtBuild(t *testing.T) {
	// No t.Parallel: the test replaces os.Stderr and changes the working
	// directory.
	registerTestAdapters()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zever.yaml"), []byte("db:\n  adapter: sqlite\n"), 0o600); err != nil {
		t.Fatalf("write zever.yaml: %v", err)
	}

	t.Chdir(dir)

	var c *Container

	atNew := captureStderr(t, func() {
		c = New(nil, Provide(coredb.DefaultRef(), dbsqlite.New))
	})
	mustCloseContainer(t, c)

	if atNew != "" {
		t.Fatalf("New wrote to stderr: %q, want none (the environment is read at build time)", atNew)
	}

	var err error

	atBuild := captureStderr(t, func() {
		_, err = Get(c, coredb.DefaultRef())
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if !strings.Contains(atBuild, "zever.yaml found in working directory") {
		t.Fatalf("provider build wrote %q, want the legacy-config warning", atBuild)
	}
}

// TestProviderEnvironmentReadOnce proves the environment is read once per
// container: two providers resolve from a single snapshot, so the
// legacy-config warning is printed once, not once per provider.
func TestProviderEnvironmentReadOnce(t *testing.T) {
	// No t.Parallel: the test replaces os.Stderr and changes the working
	// directory.
	registerTestAdapters()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zever.yaml"), []byte("db:\n  adapter: sqlite\n"), 0o600); err != nil {
		t.Fatalf("write zever.yaml: %v", err)
	}

	t.Chdir(dir)

	c := New(nil,
		Provide(coredb.DefaultRef(), dbsqlite.New),
		Provide(coredb.Ref("analytics"), dbsqlite.New))
	mustCloseContainer(t, c)

	var err error

	out := captureStderr(t, func() {
		_, err = Get(c, coredb.DefaultRef())
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	out += captureStderr(t, func() {
		_, err = Get(c, coredb.Ref("analytics"))
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got := strings.Count(out, "zever.yaml found in working directory"); got != 1 {
		t.Fatalf("legacy-config warnings = %d, want 1 (the environment is read once): %q", got, out)
	}
}

// TestNewWithoutProvidersIgnoresUnreadableEnvironment proves the legacy
// path keeps working when the environment cannot be read: a broken .env
// fails the provider builds, and the zero-provider container still resolves
// its services from the config it was given.
func TestNewWithoutProvidersIgnoresUnreadableEnvironment(t *testing.T) {
	// No t.Parallel: the test changes the working directory.
	registerTestAdapters()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_PATH=\"unclosed\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Chdir(dir)

	c := New(nil)
	mustCloseContainer(t, c)

	if _, err := c.DB(); err != nil {
		t.Fatalf("DB with an unreadable environment = %v, want nil (the legacy path never reads the environment)", err)
	}
}
