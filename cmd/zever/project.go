package main

import (
	"os"
	"strings"
)

// Project layout env keys. Project layout comes from compiled-in defaults
// below, overridable per-field by these variables. Service batteries stay
// env-only via config.FromEnv; these six keys are the sole project-settings
// surface (no --config file flag and no file discovery: they are read from
// the process environment).
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

// ProjectConfig holds CLI-only project layout conventions: where a project
// keeps its .zen schemas, its generated code, and the entrypoint packages the
// launcher subcommands (serve, worker, db seed, tinker) shell out to.
//
// It is deliberately separate from config.Config, which resolves service
// batteries from environment only. Layout resolves defaults < ZEVER_* env;
// per-command flags (--entry and friends) override the resolved entrypoint
// where the command defines one.
type ProjectConfig struct {
	SchemaDir    string `json:"schema_dir"`
	GeneratedDir string `json:"generated_dir"`
	ServerEntry  string `json:"server_entry"`
	WorkerEntry  string `json:"worker_entry"`
	TinkerEntry  string `json:"tinker_entry"`
	SeedEntry    string `json:"seed_entry"`
}

// Default project conventions. They apply unconditionally: layout is not
// read from any file. `zever new` scaffolds exactly this layout.
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
// defaults overlaid by the ZEVER_* environment (whitespace-only values count
// as unset). It performs no file discovery and no parsing: configuration
// comes from the environment (config.FromEnv warns once when a pre-v0.8.0
// config file sits in the cwd).
//
//nolint:unparam // the error return is reserved for future fallible sources.
func loadProjectConfig() (ProjectConfig, error) {
	pc := ProjectConfig{
		SchemaDir:    strings.TrimSpace(os.Getenv(EnvSchemaDir)),
		GeneratedDir: strings.TrimSpace(os.Getenv(EnvGeneratedDir)),
		ServerEntry:  strings.TrimSpace(os.Getenv(EnvServerEntry)),
		WorkerEntry:  strings.TrimSpace(os.Getenv(EnvWorkerEntry)),
		TinkerEntry:  strings.TrimSpace(os.Getenv(EnvTinkerEntry)),
		SeedEntry:    strings.TrimSpace(os.Getenv(EnvSeedEntry)),
	}

	return pc.withDefaults(), nil
}
