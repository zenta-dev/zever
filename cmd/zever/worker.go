package main

import "flag"

// WorkerConfig is the resolved configuration for one `zever queue:work` run.
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

// runQueueWork launches the project's worker entrypoint. See launch.go for
// why this shells out instead of running a worker in-process.
func runQueueWork(args []string) error {
	if hasHelpFlag(args) {
		printLauncherHelp(flag.CommandLine.Output(), "zever queue:work", "Starts this project's background worker by running its worker entrypoint package (project.worker_entry, default cmd/worker). Every argument is passed through.", "zever queue:work --once")
		return nil
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	return execLaunch(resolveWorkerConfig(cfg, args).Entry, args)
}
