package container

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
)

// TestProvideEnvSeedsOptions resolves Options from the process environment:
// DB_PATH set in the environment reaches the factory with no WithOptions.
func TestProvideEnvSeedsOptions(t *testing.T) {
	// No t.Parallel: the test mutates the process environment.

	path := filepath.Join(t.TempDir(), "env.db")
	t.Setenv("DB_PATH", path)

	c := New(nil, Provide(coredb.DefaultRef(), dbsqlite.New))
	mustCloseContainer(t, c)

	d, err := Get(c, coredb.DefaultRef())
	if err != nil {
		t.Fatalf("Get = %v, want nil", err)
	}

	if err := d.Ping(t.Context()); err != nil {
		t.Fatalf("Ping = %v, want nil", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s = %v, want the env-selected database file", path, err)
	}
}

// TestProvideEnvInvalidFailsBuild surfaces an unparsable env value as a
// resolution error (fail-closed), retryable on the next Get.
func TestProvideEnvInvalidFailsBuild(t *testing.T) {
	// No t.Parallel: the test mutates the process environment.

	t.Setenv("DB_MAXCONNS", "not-an-int")

	calls := 0
	c := New(nil, Provide(coredb.DefaultRef(), func(o coredb.Options) (coredb.DB, error) {
		calls++

		return dbsqlite.New(o)
	}))
	mustCloseContainer(t, c)

	if _, err := Get(c, coredb.DefaultRef()); err == nil {
		t.Fatal("Get with invalid DB_MAXCONNS = nil, want error")
	} else if !strings.Contains(err.Error(), "MAXCONNS") {
		t.Fatalf("Get error = %v, want it to name the field", err)
	}

	if calls != 0 {
		t.Fatalf("factory calls = %d, want 0 (env failure precedes the factory)", calls)
	}
}

// TestProvideEnvUnresolvableFailsBuild records an unreadable environment
// (malformed .env file) against every provider build.
func TestProvideEnvUnresolvableFailsBuild(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_PATH=\"unclosed\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Chdir(dir)

	c := New(nil, Provide(coredb.DefaultRef(), dbsqlite.New))
	mustCloseContainer(t, c)

	if _, err := Get(c, coredb.DefaultRef()); err == nil {
		t.Fatal("Get with malformed .env = nil, want env error")
	}
}

// TestProvideNamedEnvNamespace resolves a named instance from its
// SERVICE__NAME__* namespace, leaving the default namespace alone.
func TestProvideNamedEnvNamespace(t *testing.T) {
	// No t.Parallel: the test mutates the process environment.

	defPath := filepath.Join(t.TempDir(), "default.db")
	t.Setenv("DB_PATH", defPath)

	namedPath := filepath.Join(t.TempDir(), "named.db")
	t.Setenv("DB__ANALYTICS__PATH", namedPath)

	var defOpts, namedOpts coredb.Options

	c := New(nil,
		Provide(coredb.DefaultRef(), func(o coredb.Options) (coredb.DB, error) {
			defOpts = o

			return dbsqlite.New(o)
		}),
		Provide(coredb.Ref("analytics"), func(o coredb.Options) (coredb.DB, error) {
			namedOpts = o

			return dbsqlite.New(o)
		}))
	mustCloseContainer(t, c)

	if _, err := Get(c, coredb.DefaultRef()); err != nil {
		t.Fatalf("Get(default) = %v, want nil", err)
	}

	if _, err := Get(c, coredb.Ref("analytics")); err != nil {
		t.Fatalf("Get(analytics) = %v, want nil", err)
	}

	if defOpts.Path != defPath {
		t.Fatalf("default Path = %q, want %q", defOpts.Path, defPath)
	}

	if namedOpts.Path != namedPath {
		t.Fatalf("analytics Path = %q, want %q", namedOpts.Path, namedPath)
	}
}

// TestProvideEnvExplicitBeatsEnv runs WithOptions after the env overlay, so
// explicit Provide-site options win.
func TestProvideEnvExplicitBeatsEnv(t *testing.T) {
	// No t.Parallel: the test mutates the process environment.

	envPath := filepath.Join(t.TempDir(), "env.db")
	t.Setenv("DB_PATH", envPath)

	explicitPath := filepath.Join(t.TempDir(), "explicit.db")

	var got coredb.Options

	c := New(nil, Provide(coredb.DefaultRef(), func(o coredb.Options) (coredb.DB, error) {
		got = o

		return dbsqlite.New(o)
	}, WithOptions(func(o *coredb.Options) { o.Path = explicitPath })))
	mustCloseContainer(t, c)

	if _, err := Get(c, coredb.DefaultRef()); err != nil {
		t.Fatalf("Get = %v, want nil", err)
	}

	if got.Path != explicitPath {
		t.Fatalf("Path = %q, want explicit %q", got.Path, explicitPath)
	}
}
