package main

import (
	"errors"
	"flag"

	"github.com/spf13/cobra"
)

// WorkerConfig is the resolved configuration for one `zever worker` run.
type WorkerConfig struct {
	// Entry is the worker entrypoint package directory (project.worker_entry).
	Entry string
	// Args are passed through untouched to the entrypoint.
	Args []string
}

// resolveWorkerConfig builds a WorkerConfig from the project layout and CLI
// args without side effects.
func resolveWorkerConfig(project ProjectConfig, args []string) WorkerConfig {
	return WorkerConfig{Entry: project.WorkerEntry, Args: args}
}

// newWorkerCmd returns the canonical `worker` command: run the project's
// background worker entrypoint (project.worker_entry, default cmd/worker).
// The legacy `queue:work` top-level command stays registered but hidden
// (see root.go), so existing scripts keep working while --help shows only
// `worker`.
func newWorkerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worker",
		Short: "Run worker entrypoint (default cmd/worker)",
		Long: `Run this project's background worker by running its worker
entrypoint package (project.worker_entry, default cmd/worker). Every
argument is passed through to that package.`,
		Example:            `  zever worker`,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if herr := printHelpIfRequested(cmd, args); herr != nil {
				if errors.Is(herr, errHelpShown) {
					return nil
				}

				return herr
			}

			if versionRequested(cmd) {
				printCLIVersion()

				return nil
			}

			return runQueueWork(args)
		},
	}
}

// runQueueWork launches the project's worker entrypoint. See launch.go for
// why this shells out instead of running a worker in-process.
func runQueueWork(args []string) error {
	if hasHelpFlag(args) {
		printLauncherHelp(flag.CommandLine.Output(), "zever queue:work", "Starts this project's background worker by running its worker entrypoint package (project.worker_entry, default cmd/worker). Every argument is passed through to that package -- the scaffolded worker entrypoint itself accepts none by default.", "zever queue:work")
		return nil
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	return execLaunch(resolveWorkerConfig(cfg, args).Entry, args)
}
