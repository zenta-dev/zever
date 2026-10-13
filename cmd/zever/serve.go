package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ServeConfig is the resolved configuration for one `zever serve` run: the
// entrypoint package to launch and the flags to pass through untouched.
type ServeConfig struct {
	// Entry is the server entrypoint package directory (project.server_entry).
	Entry string
	// Args are passed through untouched to the entrypoint.
	Args []string
}

// resolveServeConfig builds a ServeConfig from the project layout and CLI
// args without side effects, so tests can pin the wiring without spawning a
// child process.
func resolveServeConfig(project ProjectConfig, args []string) ServeConfig {
	return ServeConfig{Entry: project.ServerEntry, Args: args}
}

// runServe launches the project's server entrypoint. See launch.go for why
// this shells out instead of serving in-process.
func runServe(args []string) error {
	if hasHelpFlag(args) {
		printLauncherHelp(flag.CommandLine.Output(), "zever serve", "Starts this project's HTTP server by running its server entrypoint package (project.server_entry, default cmd/server). Every argument is passed through untouched to that entrypoint.", "zever serve -addr :9090  # => go run ./cmd/server -addr :9090")
		return nil
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	serveCfg := resolveServeConfig(cfg, args)
	if err := preflightServe(serveCfg.Entry); err != nil {
		return err
	}

	return execLaunch(serveCfg.Entry, args)
}

// preflightServe fails fast with a one-line fix instead of raw `go run`
// output when the project cannot boot: missing entrypoint, missing go.sum
// (needs `go mod tidy`), or a zever.yaml battery with no Register in
// internal/app/app.go (needs `zever add <battery>`).
func preflightServe(entry string) error {
	const tag = "zever serve"

	if fi, err := os.Stat(entry); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s: entrypoint %q not found, run `zever generate server` to scaffold it", tag, entry)
	}

	if _, err := os.Stat("go.mod"); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no go.mod in working directory, run from your project root", tag)
		}
		return fmt.Errorf("%s: stat go.mod: %w", tag, err)
	}

	if fi, err := os.Stat("go.sum"); err != nil || (err == nil && fi.IsDir()) {
		return fmt.Errorf("%s: go.sum missing, run `go mod tidy` in %q", tag, mustCwd())
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: stat go.sum: %w", tag, err)
	}

	// Unregistered-adapter check: every battery explicitly listed in
	// zever.yaml must be wired in internal/app/app.go. Only explicit
	// entries are checked, never config.Default()'s full service map.
	yamlPath := discoverServeConfig()
	if yamlPath == "" {
		return nil
	}

	services, svcErr := serveYamlBatteries(yamlPath)
	if svcErr != nil || len(services) == 0 {
		// Best-effort pre-flight: an unreadable config skips the
		// unregistered-adapter check; the real error surfaces at launch.
		return nil //nolint:nilerr // intentional best-effort skip, not swallowed failure
	}

	appGo, err := os.ReadFile(filepath.Join("internal", "app", "app.go"))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: internal/app/app.go not found, run `zever generate server` to scaffold it", tag)
		}
		return nil //nolint:nilerr // intentional best-effort skip on transient read error
	}

	for _, battery := range services {
		if _, known := allServiceAdapters[battery]; !known {
			continue
		}
		if !strings.Contains(string(appGo), battery) {
			return fmt.Errorf("%s: battery %q in %s has no Register in internal/app/app.go, run `zever add %s`", tag, battery, yamlPath, battery)
		}
	}

	return nil
}

// mustCwd returns the working directory for error hints, falling back to ".".
func mustCwd() string {
	if dir, err := os.Getwd(); err == nil {
		return dir
	}
	return "."
}

// discoverServeConfig returns the project config file if present, else "".
func discoverServeConfig() string {
	for _, name := range projectDiscoveryOrder {
		if fi, err := os.Stat(name); err == nil && !fi.IsDir() {
			return name
		}
	}
	return ""
}

// serveYamlBatteries lists top-level service keys in the project config.
func serveYamlBatteries(path string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // developer project config
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var out []string
	for k := range doc {
		if k == "project" {
			continue
		}
		out = append(out, k)
	}
	return out, nil
}
