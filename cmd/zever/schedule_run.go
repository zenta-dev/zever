package main

import "flag"

const scheduleRunHelp = `zever schedule:run [flags-for-the-worker...]

schedule:run is an alias for queue:work: this project's worker entrypoint runs
the scheduler and queue worker in one process (see cmd/worker/main.go).

That single-process design is deliberate — the scheduler dispatches onto the
same queue the worker consumes, and the two share one queue instance. Running a
separate scheduler process would duplicate the container/queue wiring and risk
two processes both calling Scheduler.Start against the same distributed lock,
so no separate scheduler entrypoint exists.`

// runScheduleRun is a documented alias for queue:work. See scheduleRunHelp.
func runScheduleRun(args []string) error {
	if hasHelpFlag(args) {
		printLauncherHelp(flag.CommandLine.Output(), "zever schedule:run", "Alias for queue:work: this project's worker entrypoint runs the scheduler and queue worker in one process. No separate scheduler entrypoint exists.", "zever schedule:run")
		return nil
	}

	return runQueueWork(args)
}
