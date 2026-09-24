package main

import "flag"

// SeedConfig is the resolved configuration for one `zever db seed` run.
type SeedConfig struct {
	// Entry is the seed entrypoint package directory (project.seed_entry).
	Entry string
	// Args are passed through untouched to the entrypoint.
	Args []string
}

// resolveSeedConfig builds a SeedConfig from the project layout and CLI args
// without side effects.
func resolveSeedConfig(project ProjectConfig, args []string) SeedConfig {
	return SeedConfig{Entry: project.SeedEntry, Args: args}
}

// runDBSeed launches the project's seed entrypoint. See launch.go for why
// this shells out instead of seeding in-process.
func runDBSeed(args []string) error {
	if hasHelpFlag(args) {
		printLauncherHelp(flag.CommandLine.Output(), "zever db seed", "Seeds this project's database by running its seed entrypoint package (project.seed_entry, default db/seed). Every argument is passed through to that package -- the scaffolded seed entrypoint itself accepts none by default.", "zever db seed")
		return nil
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	return execLaunch(resolveSeedConfig(cfg, args).Entry, args)
}
