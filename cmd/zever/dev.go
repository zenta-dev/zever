package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/zenta-dev/zever/dsl/compile"
)

const devTag = "zever dev"

// Tunables, package-level so tests can shrink them. Production values: a
// debounce long enough to coalesce the burst of events editors emit for a
// single save (write + rename + chmod), and a stop grace period matching
// what a graceful HTTP shutdown reasonably needs.
var (
	devDebounce  = 250 * time.Millisecond
	devStopGrace = 5 * time.Second
)

// Seams for tests, following the execLaunch precedent: the watch loop goes
// through these instead of calling the constructors directly, so tests can
// drive devLoop without spawning subprocesses or touching the filesystem
// watcher. devNewTimer is the debounce seam: every rebuild delay goes
// through it, so tests observe the interval without sleeping.
var (
	devStartChild  = startDevChild
	devNewWatcher  = newDevWatcher
	devNewTimer    = time.NewTimer
	devCompileFunc = devCompile
)

// DevConfig is the resolved configuration for one `zever dev` session.
type DevConfig struct {
	// SchemaDir holds the .zen schemas to compile and watch.
	SchemaDir string
	// ServerEntry is the server entrypoint package to run and watch.
	ServerEntry string
	// Args are passed through untouched to the server entrypoint.
	Args []string
	// Debounce coalesces a burst of filesystem events into one rebuild.
	Debounce time.Duration
	// StopGrace bounds graceful shutdown before the old child is killed.
	StopGrace time.Duration
}

// resolveDevConfig builds a DevConfig from the project layout and CLI args
// without side effects.
func resolveDevConfig(project ProjectConfig, args []string) DevConfig {
	return DevConfig{
		SchemaDir:   project.SchemaDir,
		ServerEntry: project.ServerEntry,
		Args:        args,
		Debounce:    devDebounce,
		StopGrace:   devStopGrace,
	}
}

func printDevUsage(out io.Writer) {
	header := title("zever dev") + dim(" — watch & restart")
	usage := bold("Usage:") + "  " + cmd("zever dev") + dim(" [flags-for-the-server...]")
	desc := dim("Watch-mode for local development. Compiles .zen schemas, starts the server,") + "\n" +
		dim("and on change recompiles and restarts. A failing schema leaves the server alone.") + "\n" +
		dim("Every argument is passed through to the server entrypoint as 'zever serve' does.")

	body := joinLines(
		header,
		"",
		usage,
		"",
		desc,
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(out, box(body))
	} else {
		_, _ = fmt.Fprintln(out, body)
	}

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, dim("Examples:"))
	_, _ = fmt.Fprintln(out, dim("  ")+cmd("zever dev")+dim("                # watch schema + cmd/server"))
	_, _ = fmt.Fprintln(out, dim("  ")+cmd("zever dev -addr :9090")+dim("    # flags passed to server"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(out, formatHint("Ctrl-C stops the watcher and the server"))
	}
}

// runDev is the `zever dev` entrypoint: it resolves the project layout,
// installs the SIGINT/SIGTERM handler that unwinds the whole session, and hands
// off to devLoop.
func runDev(args []string) error {
	if hasHelpFlag(args) {
		printDevUsage(os.Stderr)
		return nil
	}

	project, err := loadProjectConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return devLoop(ctx, project, args, os.Stdout)
}

// devLoop is the watch loop proper, split from runDev so tests can drive it
// with their own cancellable context and capture its status output. It returns
// only when ctx is cancelled or the watcher cannot be established; the child
// process is always stopped before it returns.
func devLoop(ctx context.Context, project ProjectConfig, args []string, out io.Writer) error {
	cfg := resolveDevConfig(project, args)

	watcher, err := devNewWatcher(project)
	if err != nil {
		return err
	}

	defer func() { _ = watcher.Close() }()

	// The first compile is advisory: its diagnostics are printed, but the
	// server starts either way. A project may well be runnable while a schema
	// it does not yet consume is mid-edit, and refusing to start would be a
	// worse first impression than a printed error.
	if compileErr := devCompileFunc(cfg.SchemaDir, out); compileErr != nil {
		_, _ = fmt.Fprintf(out, "%s: initial compile %s %s (see diagnostics above); starting the server anyway\n", cyan(devTag), yellow("FAILED"), failMark())
	}

	// The child deliberately does not inherit ctx: cancelling a
	// CommandContext child kills it outright, and dev's whole contract is that
	// a shutdown goes through the graceful SIGTERM path in devChild.stop, which
	// the deferred stop below and every restart both run.
	child, err := devStartChild(cfg.ServerEntry, cfg.Args, out) //nolint:contextcheck // see above
	if err != nil {
		return err
	}

	defer func() { child.stop(cfg.StopGrace) }()

	_, _ = fmt.Fprintf(out, "%s: %s %s and %s %s\n", cyan(devTag), dim("watching"), cyan(cfg.SchemaDir), dim("and"), cyan(cfg.ServerEntry)+dim(" for changes (Ctrl-C to stop)"))

	// A stopped timer that later events reset: every relevant change pushes the
	// rebuild cfg.Debounce into the future, so a burst collapses into one pass.
	// coverageProof: no drain after Stop. As of Go 1.23 (go.mod says 1.27),
	// Stop on a timer with no prior receive always returns true and discards
	// any stale tick, so !Stop() implies an empty channel and a drain would
	// block forever; the branch is unreachable without hanging. Verified
	// empirically on go1.27: Stop on a fired-but-undrained timer returns true.
	debounce := devNewTimer(cfg.Debounce)
	debounce.Stop()

	defer debounce.Stop()

	for {
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintf(out, "%s: %s\n", cyan(devTag), dim("shutting down"))
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			if !devRelevantEvent(event) {
				continue
			}

			// A newly created directory is not watched by fsnotify's parent
			// watch, so pick it up before its contents start changing.
			if event.Has(fsnotify.Create) {
				if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
					_ = addWatchTree(watcher, event.Name)
				}
			}

			debounce.Reset(cfg.Debounce)

		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return nil
			}

			_, _ = fmt.Fprintf(out, "%s: %s %v\n", cyan(devTag), red("watch error:"), watchErr)

		case <-debounce.C:
			// Same reasoning as the initial launch above: signal-driven, not
			// ctx-driven, child lifetime.
			child = devRebuildWithGrace(project, args, out, child, cfg.StopGrace) //nolint:contextcheck // see above
		}
	}
}

// devRebuild runs one debounced change pass: recompile, and on success swap the
// running child for a fresh one. It returns the child that is running when it
// finishes — the same one if the compile failed or the relaunch errored, so a
// broken edit never costs the developer a working server.
func devRebuild(project ProjectConfig, args []string, out io.Writer, child *devChild) *devChild {
	return devRebuildWithGrace(project, args, out, child, devStopGrace)
}

// devRebuildWithGrace is devRebuild with an explicit grace period so tests can
// run the rebuild path without waiting out the production timeout.
func devRebuildWithGrace(project ProjectConfig, args []string, out io.Writer, child *devChild, grace time.Duration) *devChild {
	_, _ = fmt.Fprintf(out, "%s: %s, recompiling %s...\n", cyan(devTag), dim("change detected"), cyan(project.SchemaDir))

	if err := devCompileFunc(project.SchemaDir, out); err != nil {
		_, _ = fmt.Fprintf(out, "%s: compile %s, server unchanged %s %v\n", cyan(devTag), yellow("FAILED"), failMark(), red(err.Error()))
		return child
	}

	_, _ = fmt.Fprintf(out, "%s: compile %s, restarting server... %s\n", cyan(devTag), green("OK"), successMark())

	child.stop(grace)

	next, err := devStartChild(project.ServerEntry, args, out)
	if err != nil {
		_, _ = fmt.Fprintf(out, "%s: %s: %v %s\n", cyan(devTag), red("relaunch failed"), err, failMark())
		return child
	}

	return next
}

// devCompile compiles every .zen file under the project's schema directory with
// no backends (resolution only — the fastest feedback the pipeline can give)
// and prints any diagnostics. An empty or missing schema directory compiles
// vacuously, matching compileSchemaDir's contract.
func devCompile(schemaDir string, out io.Writer) error {
	files, err := collectZenFiles(devTag, schemaDir)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		_, _ = fmt.Fprintf(out, "%s: %s %s\n", cyan(devTag), dim("no .zen files under"), cyan(schemaDir))
		return nil
	}

	_, diags := compile.Compile(files)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	if diags.HasErrors() {
		return fmt.Errorf("%s: %d schema file(s) failed to compile", devTag, len(files))
	}

	return nil
}

// devChild is a running server process plus the goroutine reaping it. The done
// channel is what makes stop's grace period observable: without a Wait already
// in flight there is no way to tell "exited gracefully" from "still wedged".
type devChild struct {
	cmd  *exec.Cmd
	done chan struct{}
}

// startDevChild launches the entrypoint and starts reaping it in the
// background.
func startDevChild(entryDir string, args []string, out io.Writer) (*devChild, error) {
	_, _ = fmt.Fprintf(out, "%s: %s %s...\n", cyan(devTag), dim("starting"), cyan(entryDir))

	cmd, err := startLaunch(entryDir, args)
	if err != nil {
		return nil, err
	}

	child := &devChild{cmd: cmd, done: make(chan struct{})}

	// Reaper exits when child process ends (closes done).
	go func() {
		// A non-zero exit is the child's business (a compile error in the
		// project's own Go source, a port already bound); `go run` has already
		// written the reason to the inherited stderr, and dev keeps watching so
		// the next save can fix it.
		_ = cmd.Wait()

		close(child.done)
	}()

	return child, nil
}

// stop asks the child to shut down gracefully and hard-kills it if it outlasts
// grace. It returns only once the process is reaped, so the caller can start a
// replacement without the two overlapping on a listening port.
func (c *devChild) stop(grace time.Duration) {
	select {
	case <-c.done:
		return
	default:
	}

	forwardSignal(c.cmd, syscall.SIGTERM)

	select {
	case <-c.done:
	case <-time.After(grace):
		killProcessGroup(c.cmd)
		<-c.done
	}
}
