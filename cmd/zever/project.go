package main

import (
	"strings"
)

// ProjectConfig holds CLI-only project layout conventions: where a project
// keeps its .zen schemas, its generated code, and the entrypoint packages the
// launcher subcommands (serve, queue:work, db seed, tinker) shell out to.
//
// It is deliberately separate from config.Config, which resolves service
// batteries from environment only. Project layout comes from compiled-in
// defaults for now; flags+env overrides arrive with the CLI UX rework.
type ProjectConfig struct {
	SchemaDir    string `json:"schema_dir"    yaml:"schema_dir"`
	GeneratedDir string `json:"generated_dir" yaml:"generated_dir"`
	ServerEntry  string `json:"server_entry"  yaml:"server_entry"`
	WorkerEntry  string `json:"worker_entry"  yaml:"worker_entry"`
	TinkerEntry  string `json:"tinker_entry"  yaml:"tinker_entry"`
	SeedEntry    string `json:"seed_entry"    yaml:"seed_entry"`
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

// loadProjectConfig returns the default project layout. It performs no file
// discovery and no parsing: project layout is defaults-only until the CLI
// UX rework wires flags+env overrides.
//
//nolint:unparam // the error return is reserved for those future overrides.
func loadProjectConfig() (ProjectConfig, error) {
	return ProjectConfig{}.withDefaults(), nil
}
