package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustWriteDotEnv writes a dotenv fixture file into dir and fails the test
// when it cannot.
func mustWriteDotEnv(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestResolveServiceEnvReadsDotenvDiscoveryAndProcessEnv pins the CLI's
// only configuration source: the .env/.env.local files discovered in the
// working directory, with the process environment on top. The precedence
// chain is .env < .env.local < process.
//
// Not parallel: the test sets process variables and changes the working
// directory, which are process-wide.
func TestResolveServiceEnvReadsDotenvDiscoveryAndProcessEnv(t *testing.T) {
	dir := t.TempDir()
	mustWriteDotEnv(t, dir, ".env", "DB_MAX_CONNS=3\nDB_ADAPTER=postgres\n")
	mustWriteDotEnv(t, dir, ".env.local", "DB_MAX_CONNS=5\nDB_MIN_CONNS=7\n")

	t.Setenv("DB_MAX_CONNS", "9")
	t.Chdir(dir)

	env, err := resolveServiceEnv()
	if err != nil {
		t.Fatalf("resolveServiceEnv: %v", err)
	}

	tests := []struct {
		key  string
		want string
	}{
		{key: "DB_MAX_CONNS", want: "9"},      // the process wins over both files
		{key: "DB_MIN_CONNS", want: "7"},      // .env.local wins over .env
		{key: "DB_ADAPTER", want: "postgres"}, // .env alone
	}

	for _, tc := range tests {
		if got := env[tc.key]; got != tc.want {
			t.Errorf("env[%q] = %q, want %q", tc.key, got, tc.want)
		}
	}
}

// TestResolveServiceConfigAppliesEnvOverDefaults pins that the CLI's
// resolved Config carries the discovered dotenv value, and that a config
// FILE in the working directory is ignored: service config is env-only, so
// the dotenv file is the source and a yaml/json/toml file changes nothing.
func TestResolveServiceConfigAppliesEnvOverDefaults(t *testing.T) {
	dir := t.TempDir()
	mustWriteDotEnv(t, dir, ".env", "DB_MAX_CONNS=7\nOUTBOX_TABLE=cli_env_table\n")
	mustWriteDotEnv(t, dir, "config.yaml", "db:\n  max_conns: 99\n")

	t.Chdir(dir)

	cfg, err := resolveServiceConfig()
	if err != nil {
		t.Fatalf("resolveServiceConfig: %v", err)
	}

	if got := cfg.DB.Options.MaxConns; got != 7 {
		t.Errorf("db max conns = %d, want 7 from .env", got)
	}

	if got := cfg.Outbox.Options.Table; got != "cli_env_table" {
		t.Errorf("outbox table = %q, want %q from .env", got, "cli_env_table")
	}
}

// TestServiceConfigCommandsRejectAConfigFileFlag pins that no command
// takes a --config file flag: service config is env-only, so naming a file
// is flag misuse (exit 2), never a config load.
func TestServiceConfigCommandsRejectAConfigFileFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func([]string) error
	}{
		{name: "doctor", run: runDoctor},
		{name: "outbox status", run: runOutboxStatus},
	}

	for _, tc := range tests {
		err := tc.run([]string{"--config", "nope.yaml"})
		if err == nil {
			t.Errorf("%s: got nil error, want an unknown-flag rejection", tc.name)

			continue
		}

		if !strings.Contains(err.Error(), "config") {
			t.Errorf("%s: error = %q, want it to name the rejected flag", tc.name, err.Error())
		}
	}
}
