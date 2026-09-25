package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// newRuntimeCmds returns the runtime subcommands: serve, dev, queue:work,
// schedule:run, tinker. Each wraps its legacy runX entry point without
// touching signal handling (launch.go) or the stdlib flag parsing inside:
// DisableFlagParsing passes raw args (including -h) straight through.
func newRuntimeCmds() []*cobra.Command {
	return []*cobra.Command{
		newServeCmd(),
		newDevCmd(),
		newQueueWorkCmd(),
		newScheduleRunCmd(),
		newTinkerCmd(),
	}
}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run server entrypoint (default cmd/server)",
		Long: `Run this project's server entrypoint package
(project.server_entry, default cmd/server). Every argument is passed
through to that package.`,
		Example:            `  zever serve`,
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

			return runServe(args)
		},
	}
}

func newDevCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dev",
		Short: "Watch schemas & source; recompile and restart",
		Long: `Watch schemas and source, then recompile and restart the
server entrypoint. Every argument is passed through to the server
entrypoint as 'zever serve' does. Local watch mode only.`,
		Example: `  zever dev
  zever dev -addr :9090`,
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

			return runDev(args)
		},
	}
}

func newQueueWorkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "queue:work",
		Short: "Run worker entrypoint (default cmd/worker)",
		Long: `Run this project's background worker by running its worker
entrypoint package (project.worker_entry, default cmd/worker). Every
argument is passed through to that package.`,
		Example:            `  zever queue:work`,
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

func newScheduleRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schedule:run",
		Short: "Alias for queue:work (worker runs scheduler)",
		Long: `Alias for queue:work: the worker entrypoint runs the
scheduler, so this runs the same worker package. Every argument is
passed through to that package.`,
		Example:            `  zever schedule:run`,
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

			return runScheduleRun(args)
		},
	}
}

func newTinkerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tinker",
		Short: "Live container REPL via tinker shim",
		Long: `Open a live container REPL (yaegi) against the project's
tinker shim. Development-only: it evaluates arbitrary Go against a
live container, so run it against a development database only.`,
		Example: `  zever tinker
  zever tinker --entry ./cmd/tinker-shim`,
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

			return runTinker(args)
		},
	}
}
