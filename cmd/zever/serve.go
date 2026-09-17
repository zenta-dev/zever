package main

import (
	"flag"
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

	return execLaunch(resolveServeConfig(cfg, args).Entry, args)
}
