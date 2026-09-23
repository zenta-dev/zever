package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ProjectConfig holds CLI-only project layout conventions: where a project
// keeps its .zen schemas, its generated code, and the entrypoint packages the
// launcher subcommands (serve, queue:work, db seed, tinker) shell out to.
//
// It is deliberately separate from config.Config: config.Load decodes every
// top-level key of the same file as a service envelope and rejects unknown
// services, so a "project" table read through that path fails. Instead this
// is its own discovery+decode pass over the same zever.yaml/.yml/.json file,
// reading only the "project" key. Both loaders therefore read one physical
// file without interfering: the project pass below ignores unknown keys,
// while config.Load owns the service tables.
//
// Adaptation note: the generated zever.yaml written by `zever new` carries
// service tables only (no "project" table), because config.Load's strict
// merge rejects a "project" key. loadProjectConfig returns the defaults
// below when no file exists or when the file has no "project" table, which
// is exactly the layout `zever new` scaffolds.
type ProjectConfig struct {
	SchemaDir    string `json:"schema_dir"    yaml:"schema_dir"`
	GeneratedDir string `json:"generated_dir" yaml:"generated_dir"`
	ServerEntry  string `json:"server_entry"  yaml:"server_entry"`
	WorkerEntry  string `json:"worker_entry"  yaml:"worker_entry"`
	TinkerEntry  string `json:"tinker_entry"  yaml:"tinker_entry"`
	SeedEntry    string `json:"seed_entry"    yaml:"seed_entry"`
}

// projectFile is the decode target: only the "project" key is read, every
// other top-level key (the service tables config.Load owns) is ignored.
type projectFile struct {
	Project ProjectConfig `json:"project" yaml:"project"`
}

// Default project conventions. They apply whenever a field is empty,
// including when no config file exists at all — the config file is entirely
// optional. `zever new` scaffolds exactly this layout.
const (
	defaultSchemaDir    = "schema"
	defaultGeneratedDir = "generated"
	defaultServerEntry  = "cmd/server"
	defaultWorkerEntry  = "cmd/worker"
	defaultTinkerEntry  = "cmd/tinker-shim"
	defaultSeedEntry    = "db/seed"
)

// projectDiscoveryOrder mirrors config.discoveryOrder: the same file names in
// the same precedence, searched in the working directory.
//
// Adaptation note: TOML is gone. zever's config package decodes YAML and
// JSON only, so the project pass matches it (the predecessor supported a TOML project file;
// there is no zever.toml).
var projectDiscoveryOrder = []string{"zever.yaml", "zever.yml", "zever.json"}

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

// loadProjectConfig discovers zever.yaml/.yml/.json in the working
// directory (first match wins), decodes its "project" table, and applies
// defaults to every field the file left empty. No config file is not an
// error: the all-defaults ProjectConfig is returned.
func loadProjectConfig() (ProjectConfig, error) {
	path, err := discoverProjectFile()
	if err != nil {
		return ProjectConfig{}, err
	}

	if path == "" {
		return ProjectConfig{}.withDefaults(), nil
	}

	pc, err := decodeProjectFile(path)
	if err != nil {
		return ProjectConfig{}, err
	}

	return pc.withDefaults(), nil
}

func discoverProjectFile() (string, error) {
	for _, name := range projectDiscoveryOrder {
		info, err := os.Stat(name)
		if err == nil && !info.IsDir() {
			return name, nil
		}

		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("[zever] stat %q: %w", name, err)
		}
	}

	return "", nil
}

func decodeProjectFile(path string) (ProjectConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is a developer-supplied config location, not user input
	if err != nil {
		return ProjectConfig{}, fmt.Errorf("[zever] read %q: %w", path, err)
	}

	var file projectFile

	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &file); err != nil {
			return ProjectConfig{}, fmt.Errorf("[zever] parse yaml %q: %w", path, err)
		}
	case ".json":
		if err := json.Unmarshal(data, &file); err != nil {
			return ProjectConfig{}, fmt.Errorf("[zever] parse json %q: %w", path, err)
		}
	default:
		return ProjectConfig{}, fmt.Errorf("[zever] %q: unsupported config file extension %q", path, ext)
	}

	return file.Project, nil
}
