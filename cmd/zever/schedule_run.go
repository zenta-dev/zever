package main

import (
	"errors"
	"flag"

	"github.com/spf13/cobra"
)

const scheduleRunHelp = `zever schedule run [flags-for-the-worker...]

schedule run is an alias for worker: this project's worker entrypoint runs
the scheduler and queue worker in one process (see cmd/worker/main.go).

That single-process design is deliberate — the scheduler dispatches onto the
same queue the worker consumes, and the two share one queue instance. Running a
separate scheduler process would duplicate the container/queue wiring and risk
two processes both calling Scheduler.Start against the same distributed lock,
so no separate scheduler entrypoint exists.`

// runScheduleRun is a documented alias for queue:work. See scheduleRunHelp.
func runScheduleRun(args []string) error {
	if hasHelpFlag(args) {
		printLauncherHelp(flag.CommandLine.Output(), "zever schedule run", "Alias for worker: this project's worker entrypoint runs the scheduler and queue worker in one process. No separate scheduler entrypoint exists.", "zever schedule run")
		return nil
	}

	return runQueueWork(args)
}

// runScheduleLegacy is the legacy-dispatch entry for the canonical
// `schedule run` spelling: it strips one leading "run" (so both `schedule`
// with no subcommand help and `schedule run ...` work) and delegates to
// runScheduleRun. Production traffic goes through the Cobra tree; this only
// serves direct run() callers.
func runScheduleLegacy(args []string) error {
	if len(args) > 0 && args[0] == "run" {
		args = args[1:]
	}

	return runScheduleRun(args)
}

// newScheduleCmd returns the canonical `schedule` parent with its `run`
// subcommand. The legacy `schedule:run` spelling stays as a hidden
// top-level command (see root.go), so existing scripts keep working while
// --help shows only the space form.
func newScheduleCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "schedule",
		Short: "Run scheduled jobs via the worker",
		Long: `Run scheduled jobs via this project's worker entrypoint.
The worker runs the scheduler and queue worker in one process.`,
		Example: `  zever schedule run`,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}

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

	run := &cobra.Command{
		Use:   "run",
		Short: "Alias for worker (worker runs scheduler)",
		Long: `Alias for worker: the worker entrypoint runs the
scheduler, so this runs the same worker package. Every argument is
passed through to that package.`,
		Example:            `  zever schedule run`,
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

	parent.AddCommand(run)

	return parent
}
