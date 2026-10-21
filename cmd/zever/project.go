package main

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/config"
)

// Project layout env keys. Project layout comes from compiled-in defaults
// below, overridable per-field by these variables. They resolve with the
// framework's precedence -- defaults < .env < .env.local < process
// environment (see config.FromEnv) -- so a project configures its layout in
// the same file as the rest of its settings. Service batteries stay env-only
// via config.FromEnv; these six keys are the sole project-settings surface
// (no --config file flag and no file discovery).
const (
	// EnvSchemaDir overrides the .zen schema directory.
	EnvSchemaDir = "ZEVER_SCHEMA_DIR"
	// EnvGeneratedDir overrides the generated-code directory.
	EnvGeneratedDir = "ZEVER_GENERATED_DIR"
	// EnvServerEntry overrides the server entrypoint package.
	EnvServerEntry = "ZEVER_SERVER_ENTRY"
	// EnvWorkerEntry overrides the worker entrypoint package.
	EnvWorkerEntry = "ZEVER_WORKER_ENTRY"
	// EnvTinkerEntry overrides the tinker shim entrypoint package.
	EnvTinkerEntry = "ZEVER_TINKER_ENTRY"
	// EnvSeedEntry overrides the seed entrypoint package.
	EnvSeedEntry = "ZEVER_SEED_ENTRY"
)

// projectEnvHint names where the ZEVER_* layout keys live: the .env at the
// project root, or the process environment. The framework resolves them
// defaults < .env < .env.local < process environment, so a key belongs in
// the .env the project already keeps its settings in (the .env.local next to
// it overrides it) unless the shell sets it. Every help text that names one
// of these keys repeats this hint, so the help cannot promise a file the
// CLI does not read.
const projectEnvHint = ".env at the project root, or the process environment"

// ProjectConfig holds CLI-only project layout conventions: where a project
// keeps its .zen schemas, its generated code, and the entrypoint packages the
// launcher subcommands (serve, worker, db seed, tinker) shell out to.
//
// It is deliberately separate from config.Config, which resolves service
// batteries from environment only. Layout resolves defaults < .env < .env.local
// < process environment; per-command flags (--entry and friends) override the
// resolved entrypoint where the command defines one.
type ProjectConfig struct {
	SchemaDir    string `json:"schema_dir"`
	GeneratedDir string `json:"generated_dir"`
	ServerEntry  string `json:"server_entry"`
	WorkerEntry  string `json:"worker_entry"`
	TinkerEntry  string `json:"tinker_entry"`
	SeedEntry    string `json:"seed_entry"`
}

// Default project conventions. They apply when nothing overrides them:
// layout is read from the environment, never from a config file. `zever new`
// scaffolds exactly this layout.
const (
	defaultSchemaDir    = "schema"
	defaultGeneratedDir = "generated"
	defaultServerEntry  = "cmd/server"
	defaultWorkerEntry  = "cmd/worker"
	defaultTinkerEntry  = "cmd/tinker-shim"
	defaultSeedEntry    = "db/seed"
)

// withDefaults returns pc with every empty field filled in with its default.
func (pc ProjectConfig) withDefaults() ProjectConfig {
	fill := func(v, def string) string {
		if strings.TrimSpace(v) == "" {
			return def
		}

		return v
	}

	pc.SchemaDir = fill(pc.SchemaDir, defaultSchemaDir)
	pc.GeneratedDir = fill(pc.GeneratedDir, defaultGeneratedDir)
	pc.ServerEntry = fill(pc.ServerEntry, defaultServerEntry)
	pc.WorkerEntry = fill(pc.WorkerEntry, defaultWorkerEntry)
	pc.TinkerEntry = fill(pc.TinkerEntry, defaultTinkerEntry)
	pc.SeedEntry = fill(pc.SeedEntry, defaultSeedEntry)

	return pc
}

// loadProjectConfig returns the effective project layout: compiled-in
// defaults overlaid by the ZEVER_* environment, resolved with the same
// precedence the rest of the framework uses -- defaults < .env < .env.local
// < process environment (config.FromEnv). Whitespace-only values count as
// unset.
//
// The dotenv files are the ones in the working directory: that is the
// project root the CLI already resolves its project-relative paths from
// (the go.mod preflight asks for the project root by name).
//
// A malformed .env or .env.local fails closed, naming the file and the line,
// like every other env read in the framework.
func loadProjectConfig() (ProjectConfig, error) {
	env, err := config.FromEnv()
	if err != nil {
		return ProjectConfig{}, fmt.Errorf("zever: resolve project layout: %w", err)
	}

	pc := ProjectConfig{
		SchemaDir:    strings.TrimSpace(env[EnvSchemaDir]),
		GeneratedDir: strings.TrimSpace(env[EnvGeneratedDir]),
		ServerEntry:  strings.TrimSpace(env[EnvServerEntry]),
		WorkerEntry:  strings.TrimSpace(env[EnvWorkerEntry]),
		TinkerEntry:  strings.TrimSpace(env[EnvTinkerEntry]),
		SeedEntry:    strings.TrimSpace(env[EnvSeedEntry]),
	}

	return pc.withDefaults(), nil
}
