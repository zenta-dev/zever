package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
)

// Launcher pattern.
//
// zever registers jobs, routes and schedules as compiled Go closures inside
// the running binary (job.Register stores handlers in a process-local map;
// routes are hand-wired Go code). The zever binary therefore cannot generically
// *be* a project's server or worker — it can only start one. So serve,
// queue:work, schedule:run and db seed all shell out to `go run <entryDir>` by
// project convention, exactly the way `rails server` launches a separate Puma
// process rather than reimplementing an HTTP server inside the gem.
//
// Signal handling. The reference entrypoints shut down gracefully off
// signal.NotifyContext, so the launcher must deliver SIGINT/SIGTERM to them
// rather than let them be killed outright. This is subtler than it looks,
// because `go run` is not a transparent wrapper: it *ignores* SIGINT itself
// and relies on the terminal delivering the signal to the whole foreground
// process group, so signalling the `go run` pid alone reaches nothing
// (verified — the child never sees it).
//
// So runLaunch puts `go run` and the binary it compiles into their own process
// group and signals the group. That is a Unix mechanism; see launch_unix.go /
// launch_other.go. Windows signal delivery is unreliable and explicitly out of
// scope for this CLI — on non-Unix the launcher degrades to signalling the
// immediate child directly.

// execLaunch is the indirection point every launcher command calls. It is a
// package-level variable so tests can stub the shell-out.
var execLaunch = runLaunch

// hasHelpFlag reports whether the launcher was asked for its own usage. It
// scans all args so a help flag anywhere triggers the launcher help.
func hasHelpFlag(args []string) bool {
	for _, a := range args {
		switch a {
		case "-h", "--help", "help":
			return true
		}
	}

	return false
}

// printLauncherHelp writes a styled launcher command's usage text to out.
// It uses title+dim+bold+box helpers so all launchers share the same look.
func printLauncherHelp(out io.Writer, name, desc, example string) {
	header := title(name) + dim(" — "+descShort(name))
	usage := bold("Usage:") + "  " + cmd(name) + dim(" [flags-for-the-entry...]")

	body := joinLines(
		header,
		"",
		usage,
		"",
		dim(desc),
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(out, box(body))
	} else {
		_, _ = fmt.Fprintln(out, body)
	}

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, dim("Example:"))
	_, _ = fmt.Fprintln(out, "  "+cmd(example))
	_, _ = fmt.Fprintln(out, dim("SIGINT/SIGTERM are forwarded to the child for graceful shutdown."))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(out, formatHint("run '"+name+" -h' for entrypoint flags (passed through)"))
	}
}

func descShort(name string) string {
	switch name {
	case "zever serve":
		return "run server entrypoint"
	case "zever queue:work":
		return "run worker entrypoint"
	case "zever schedule:run":
		return "run scheduler (alias for queue:work)"
	case "zever db seed":
		return "seed database"
	case "zever dev":
		return "watch & restart"
	default:
		return "launcher"
	}
}

// runLaunch runs `go run <entryDir> <passthroughArgs...>` with stdin, stdout
// and stderr wired straight through to the parent process, forwarding
// SIGINT/SIGTERM to the child so its own graceful-shutdown path runs.
func runLaunch(entryDir string, passthroughArgs []string) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	defer signal.Stop(sigCh)

	cmd, err := startLaunch(entryDir, passthroughArgs)
	if err != nil {
		return err
	}

	var signalled atomic.Bool

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case sig := <-sigCh:
				signalled.Store(true)

				forwardSignal(cmd, sig)
			case <-done:
				return
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		// Group delivery reaches `go run` as well as the binary it spawned,
		// and cmd/go exits non-zero when interrupted even though the child
		// shut down cleanly. A shutdown the operator asked for is not a
		// launcher failure, so an exit status after a forwarded signal is
		// reported as success.
		var exitErr *exec.ExitError
		if signalled.Load() && errors.As(err, &exitErr) {
			return nil
		}

		return fmt.Errorf("zever: launch %q: %w", entryDir, err)
	}

	return nil
}

// startLaunch starts `go run <entryDir> <passthroughArgs...>` in its own
// process group with the parent's stdio wired through, and returns the running
// command *without* waiting for it.
//
// It exists for `zever dev`, which has to keep a handle on the child so a file
// change can stop and relaunch it. runLaunch (the blocking variant every other
// launcher command uses, and whose behaviour must not change) is layered on top
// of it. Callers of startLaunch own the child: they must eventually Wait on it,
// and they install their own signal handling — startLaunch installs none.
func startLaunch(entryDir string, passthroughArgs []string) (*exec.Cmd, error) {
	// A never-cancelled context: the child's lifetime is governed by forwarded
	// signals, not by a deadline. entryDir comes from the developer's own zever
	// config, not from untrusted input.
	//nolint:gosec // see above
	cmd := exec.CommandContext(context.Background(), "go", buildGoRunArgs(entryDir, passthroughArgs)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	isolateProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("zever: launch %q: %w", entryDir, err)
	}

	return cmd, nil
}

// buildGoRunArgs builds the argv for `go run <entry> <args...>` as an array.
// It never invokes a shell: entryDir and passthrough args stay separate
// elements so no quoting or interpolation can reinterpret them.
func buildGoRunArgs(entryDir string, passthroughArgs []string) []string {
	argv := make([]string, 0, len(passthroughArgs)+2)
	argv = append(argv, "run", goRunPath(entryDir))
	argv = append(argv, passthroughArgs...)

	return argv
}

// goRunPath turns a project entry directory into something `go run` reads as a
// filesystem path rather than an import path. The configured entries are
// directory conventions ("cmd/server", "db/seed"), and a bare "cmd/server"
// makes the go tool look for a *standard library* package of that name
// ("package cmd/server is not in std"). Prefixing "./" is what disambiguates.
func goRunPath(entryDir string) string {
	slashed := filepath.ToSlash(entryDir)

	switch {
	case slashed == "":
		return "."
	case filepath.IsAbs(entryDir):
		return entryDir
	case strings.HasPrefix(slashed, "./"), strings.HasPrefix(slashed, "../"),
		slashed == ".", slashed == "..":
		return slashed
	default:
		return "./" + slashed
	}
}
