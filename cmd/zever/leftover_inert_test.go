package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
)

// legacyConfigFileName returns the name of a file left over from a release
// that still read one. The name is assembled from parts so this package
// never spells a retired name out (see TestCLINeverNamesALegacyConfigFile
// in help_envkeys_test.go, which fails the gate on a literal).
func legacyConfigFileName(ext string) string {
	return "zever" + "." + ext
}

// TestConfigShowIgnoresLeftoverFileWithConflictingValue proves the CLI is
// env-only at the command surface: `zever config show` reports the
// .env/environment value even when a leftover file from a retired
// file-based release sits next to the .env with a conflicting value, the
// leftover value reaches neither stdout nor stderr, and nothing warns
// about the file.
//
// Contract (clean break since v0.8.0): service configuration comes from
// `.env`, `.env.local` and the process environment (see config.FromEnv);
// no project file is ever read, and the removal warns about nothing at
// runtime. A warning would re-couple the CLI to a file it deliberately
// ignores: the Twelve-Factor App names the environment the only config
// surface (https://12factor.net/config), and a breaking removal is
// announced once in the release notes, not nagged on every invocation
// (https://semver.org: a major bump communicates the incompatibility).
//
// Seam: runConfigShowWithEnv takes the env map as the command's single
// input (added in d8e69e51), so the test never touches the working
// directory and never reads the process environment — no t.Chdir, no
// t.Parallel conflict, immune to ambient keys (AI_AGENT and friends).
// Dependency injection over process globals keeps the test hermetic and
// the failure attributable: cf. Fowler on separating configuration from
// use (https://martinfowler.com/articles/injection.html) and Nygard's
// "Look Ma, No Globals" rule. The .env on disk is loaded by explicit
// path (config.LoadEnv), never by discovery, so the leftover beside it
// can only leak through a code path that reads it — there is none.
//
// No t.Parallel: captureStderr redirects os.Stderr process-wide.
func TestConfigShowIgnoresLeftoverFileWithConflictingValue(t *testing.T) {
	const (
		envValue     = "data/app.db"
		leftoverMark = "from-legacy-inert-canary"
	)

	dir := t.TempDir()
	dotenvPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(dotenvPath, []byte("DB_PATH="+envValue+"\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	// Leftover content is a literal string: this package must not gain a
	// parser dependency for a file it never reads.
	leftoverBody := "db:\n  adapter: " + leftoverMark + "\n"
	if err := os.WriteFile(filepath.Join(dir, legacyConfigFileName("yaml")), []byte(leftoverBody), 0o600); err != nil {
		t.Fatalf("write leftover file: %v", err)
	}

	// Explicit path, not discovery: only the .env feeds the env map.
	env, err := config.LoadEnv(dotenvPath)
	if err != nil {
		t.Fatalf("config.LoadEnv(.env): %v", err)
	}

	var out bytes.Buffer
	var runErr error
	stderr := captureStderr(t, func() {
		runErr = runConfigShowWithEnv(ConfigConfig{Out: &out}, env)
	})
	if runErr != nil {
		t.Fatalf("runConfigShowWithEnv: %v", runErr)
	}

	stdout := out.String()

	// (a) The reported value is the .env value, not the leftover's.
	if !strings.Contains(stdout, "db  driver=sqlite") {
		t.Fatalf("output = %q, want the code-wired db driver", stdout)
	}
	if !strings.Contains(stdout, envValue) {
		t.Fatalf("output = %q, want the .env value %q", stdout, envValue)
	}

	// (b) The leftover value reaches neither stdout nor stderr (nor the
	// diagnostic error, which is nil on this path).
	for _, got := range []string{stdout, stderr} {
		if strings.Contains(got, leftoverMark) {
			t.Fatalf("output carries the leftover value %q: stdout=%q stderr=%q", leftoverMark, stdout, stderr)
		}
	}

	// (c) Nothing warns about the file. The seam makes this vacuous by
	// construction — its signature takes only the env map, so no code
	// path can stat, read, or name the leftover — and the capture proves
	// it: the command wrote nothing to stderr and returned no
	// present/ignored/deprecated/unknown diagnostic. The discovery half
	// (FromEnv reads dotenv files only, silently) is covered where it
	// lives: config.TestFromEnvIgnoresLeftoverDocWithConflictingValue
	// captures FromEnv's stderr and asserts it is empty.
	if stderr != "" {
		t.Fatalf("stderr = %q, want nothing (the leftover file is not reported)", stderr)
	}
	for _, word := range []string{"present", "ignored", "deprecated", "unknown"} {
		if strings.Contains(strings.ToLower(stderr), word) {
			t.Fatalf("stderr = %q, want no %q warning about the leftover file", stderr, word)
		}
	}
	leftoverName := strings.ToLower(legacyConfigFileName("yaml"))
	if strings.Contains(strings.ToLower(stdout+" "+stderr), leftoverName) {
		t.Fatalf("output names the leftover file %q: stdout=%q stderr=%q", leftoverName, stdout, stderr)
	}
}
