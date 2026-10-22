package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadEnvRejectsNonDotenvDocument proves LoadEnv parses dotenv files
// ONLY: a YAML, JSON or TOML document is not a dotenv file, so handing one
// to LoadEnv fails closed with a file-and-line error instead of a partial
// map. Silently ignoring the unknown format would turn a typo into a
// missing setting, so the loader names the exact file and line.
func TestLoadEnvRejectsNonDotenvDocument(t *testing.T) {
	t.Parallel()

	const canary = "s3cr3t-nondotenv-canary"
	tests := []struct {
		name    string
		ext     string
		content string
	}{
		{
			name:    "yaml",
			ext:     "yaml",
			content: "server:\n  adapter: " + canary + "\n",
		},
		{
			name:    "json",
			ext:     "json",
			content: "{\n  \"server\": \"" + canary + "\"\n}\n",
		},
		{
			name:    "toml",
			ext:     "toml",
			content: "[server]\nadapter = \"" + canary + "\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			p := filepath.Join(dir, "doc."+tt.ext)
			if err := os.WriteFile(p, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			env, err := LoadEnv(p)
			if err == nil {
				t.Fatalf("LoadEnv(%q) = %v, want error: a %s document is not a dotenv file", p, env, tt.name)
			}
			if !strings.HasPrefix(err.Error(), "config:") {
				t.Fatalf("error must carry the config: prefix: %v", err)
			}
			if !strings.Contains(err.Error(), "doc."+tt.ext+":1") {
				t.Fatalf("error must name file and line: %v", err)
			}
			if strings.Contains(err.Error(), canary) {
				t.Fatalf("error echoes value: %v", err)
			}
			if len(env) != 0 {
				t.Fatalf("LoadEnv returned a partial map: %v", env)
			}
		})
	}
}

// TestFromEnvIgnoresLeftoverDocWithConflictingValue proves FromEnv reads
// dotenv files only: a leftover document from a retired file-based release
// sits next to a .env that sets the same key to a different value, and the
// .env value wins while the leftover value never reaches the environment.
// Discovery reports nothing: no error, no stderr warning.
//
// No t.Parallel: the test changes the working directory, and t.Chdir is
// process-wide (pkg.go.dev/testing#T.Chdir: "Because Chdir affects the
// whole process, it cannot be used in parallel tests or tests with parallel
// ancestors").
func TestFromEnvIgnoresLeftoverDocWithConflictingValue(t *testing.T) {
	const key = "DOTENV_LEFTOVER_CONFLICT_KEY"
	dir := t.TempDir()
	// The leftover name is assembled by legacyConfigFileName so this file
	// never spells a retired name out.
	leftover := "DOTENV_LEFTOVER_CONFLICT_KEY: from-legacy\n"
	if err := os.WriteFile(filepath.Join(dir, legacyConfigFileName("yaml")), []byte(leftover), 0o600); err != nil {
		t.Fatal(err)
	}
	writeDotEnv(t, dir, ".env", key+"=from-env\n")
	t.Chdir(dir)

	var env Env
	var err error
	out := captureStderr(t, func() {
		env, err = FromEnv()
	})
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if out != "" {
		t.Fatalf("FromEnv wrote %q, want nothing (the leftover file is not reported)", out)
	}
	if env[key] != "from-env" {
		t.Fatalf("env[%q] = %q, want %q", key, env[key], "from-env")
	}
	for k, v := range env {
		if strings.Contains(v, "from-legacy") {
			t.Fatalf("leftover value leaked into env[%q]", k)
		}
	}
}
