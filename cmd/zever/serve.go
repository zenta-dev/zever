package main

import (
	"flag"
	"fmt"
	"os"
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
// output when the project cannot boot: missing entrypoint or missing go.sum
// (needs `go mod tidy`).
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

	return nil
}

// mustCwd returns the working directory for error hints, falling back to ".".
func mustCwd() string {
	if dir, err := os.Getwd(); err == nil {
		return dir
	}
	return "."
}
