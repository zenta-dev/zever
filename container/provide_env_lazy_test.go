package container

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
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

// legacyConfigFileName returns the name of a config file left over from a
// release that still had one. The name is assembled from parts so this
// package never spells a legacy name out: the framework must not know
// them.
func legacyConfigFileName(ext string) string {
	return "zever" + "." + ext
}

// writeLegacyConfigFile writes a config file left over from an older
// release into dir. Its stanza names a connection limit and a DSN that
// the environment never sets, so a file that was still read would change
// the resolved options.
func writeLegacyConfigFile(t *testing.T, dir, ext string) {
	t.Helper()

	name := legacyConfigFileName(ext)
	body := "db:\n  adapter: postgres\n  options:\n    dsn: legacy-dsn\n    max_conns: 4242\n"

	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestNewWithoutProvidersSkipsEnvironment proves the zero-provider path
// never reads the process environment: the container does not read the
// working directory, so a config file left over from an older release
// cannot reach it, and nothing is written to stderr.
func TestNewWithoutProvidersSkipsEnvironment(t *testing.T) {
	// No t.Parallel: the test replaces os.Stderr and changes the working
	// directory.
	dir := t.TempDir()
	writeLegacyConfigFile(t, dir, "yaml")

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
// reads the process environment when the first provider is built, and that
// a config file left over from an older release is inert: the build
// succeeds, writes nothing to stderr, and the environment alone decides
// the options.
func TestNewWithProvidersReadsEnvironmentAtBuild(t *testing.T) {
	// No t.Parallel: the test replaces os.Stderr and changes the working
	// directory.
	registerTestAdapters()

	for _, ext := range []string{"yaml", "yml", "json", "toml"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			writeLegacyConfigFile(t, dir, ext)

			t.Chdir(dir)
			t.Setenv("ZEVER_DEBUG", "")

			dbPath := filepath.Join(dir, "env.db")
			t.Setenv("DB_PATH", dbPath)

			var opts coredb.Options

			c := New(nil, Provide(coredb.DefaultRef(), func(o coredb.Options) (coredb.DB, error) {
				opts = o

				return dbsqlite.New(o)
			}))
			mustCloseContainer(t, c)

			if c.env != nil {
				t.Fatal("New read the environment, want the read to happen at build time")
			}

			var err error

			atBuild := captureStderr(t, func() {
				_, err = Get(c, coredb.DefaultRef())
			})
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if atBuild != "" {
				t.Fatalf("provider build wrote %q, want nothing (a leftover config file is not reported)", atBuild)
			}
			if c.env == nil {
				t.Fatal("the provider build did not read the environment")
			}
			if opts.Path != dbPath {
				t.Fatalf("db path = %q, want %q (the environment decides)", opts.Path, dbPath)
			}
			if opts.DSN == "legacy-dsn" || opts.MaxConns == 4242 {
				t.Fatalf("db options = %+v, want the leftover config file to be inert", opts)
			}
		})
	}
}

// TestProviderEnvironmentReadOnce proves the environment is read once per
// container: two providers resolve from a single snapshot, so a change to
// the environment between the two resolutions is not picked up. A config
// file left over from an older release changes nothing: neither build
// writes to stderr.
func TestProviderEnvironmentReadOnce(t *testing.T) {
	// No t.Parallel: the test replaces os.Stderr and changes the working
	// directory.
	registerTestAdapters()

	for _, ext := range []string{"yaml", "yml", "json", "toml"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			writeLegacyConfigFile(t, dir, ext)

			t.Chdir(dir)
			t.Setenv("ZEVER_DEBUG", "")

			first := filepath.Join(dir, "first.db")
			second := filepath.Join(dir, "second.db")

			t.Setenv("DB_PATH", first)
			t.Setenv("DB__ANALYTICS__PATH", first)

			var defaultOpts, analyticsOpts coredb.Options

			c := New(nil,
				Provide(coredb.DefaultRef(), func(o coredb.Options) (coredb.DB, error) {
					defaultOpts = o

					return dbsqlite.New(o)
				}),
				Provide(coredb.Ref("analytics"), func(o coredb.Options) (coredb.DB, error) {
					analyticsOpts = o

					return dbsqlite.New(o)
				}))
			mustCloseContainer(t, c)

			var err error

			out := captureStderr(t, func() {
				_, err = Get(c, coredb.DefaultRef())
			})
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if out != "" {
				t.Fatalf("provider build wrote %q, want nothing (a leftover config file is not reported)", out)
			}
			if defaultOpts.Path != first {
				t.Fatalf("default db path = %q, want %q", defaultOpts.Path, first)
			}

			// The environment was read once, at the first build: a change
			// now is not picked up by the second provider.
			t.Setenv("DB_PATH", second)
			t.Setenv("DB__ANALYTICS__PATH", second)

			out += captureStderr(t, func() {
				_, err = Get(c, coredb.Ref("analytics"))
			})
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if out != "" {
				t.Fatalf("provider build wrote %q, want nothing (a leftover config file is not reported)", out)
			}
			if analyticsOpts.Path != first {
				t.Fatalf("analytics db path = %q, want %q (the environment is read once)", analyticsOpts.Path, first)
			}
			if got := c.env["DB__ANALYTICS__PATH"]; got != first {
				t.Fatalf("env snapshot = %q, want %q (the environment is read once)", got, first)
			}
			if defaultOpts.DSN == "legacy-dsn" || analyticsOpts.DSN == "legacy-dsn" {
				t.Fatal("the leftover config file reached the options")
			}
		})
	}
}

// TestNewWithoutProvidersIgnoresUnreadableEnvironment proves the
// config-based path keeps working when the environment cannot be read: a
// broken .env fails the provider builds, and the zero-provider container
// still resolves its services from the config it was given.
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
		t.Fatalf("DB with an unreadable environment = %v, want nil (the config-based path never reads the environment)", err)
	}
}

// TestPackageSourceHasNoLegacyConfigNames guards the clean break: no file
// in this package may name a config file left over from an older release,
// so the framework can neither stat such a file nor warn about it. The
// pattern is assembled from parts so this file does not spell a legacy
// name out either.
func TestPackageSourceHasNoLegacyConfigNames(t *testing.T) {
	t.Parallel()

	exts := []string{"json", "yml", "yaml", "toml"}
	pattern := regexp.MustCompile(`(?i)zever\.(` + strings.Join(exts, "|") + `)`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if loc := pattern.FindIndex(data); loc != nil {
			t.Errorf("%s names a legacy config file at offset %d", entry.Name(), loc[0])
		}
	}
}
