package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/dsl/backend/atlas"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/orm/migrate"
	"github.com/zenta-dev/zever/serve"
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
// through it, so tests observe the interval without sleeping. The boot
// sequence seams (devGenerateFunc, devMigrateFunc, devRoutesFunc) and the
// prompt seam exist for the same reason: devPrepare runs before every watch
// loop, and tests must drive it without writing files, migrating databases,
// or needing a TTY.
var (
	devStartChild   = startDevChild
	devNewWatcher   = newDevWatcher
	devNewTimer     = time.NewTimer
	devCompileFunc  = devCompile
	devGenerateFunc = devGenerate
	devGenerateFull = GenerateFull
	devMigrateFunc  = devAutoMigrate
	devRoutesFunc   = devPrintRoutes
	// promptConfirmForDev isolates the auto-migrate prompt: huh needs a
	// real TTY, so tests stub this seam to simulate answers.
	promptConfirmForDev = promptConfirm
)

// DefaultDevDBPath mirrors the scaffolded app.DefaultDBPath: the sqlite
// file dev auto-migrates when the resolved config names no path of its own.
const DefaultDevDBPath = "data/app.db"

// devEnvKey marks the child process as a non-production dev boot: serve
// refuses the deterministic dev crypto key without a dev ZEVER_ENV, and
// `zever new`'s .env is not auto-loaded by the generated entrypoint.
const devEnvKey = "ZEVER_ENV=dev"

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
		dim("-addr and -grpc-addr configure the listen addresses; every other argument") + "\n" +
		dim("is passed through to the server entrypoint as 'zever serve' does.")

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
	_, _ = fmt.Fprintln(out, dim("  ")+cmd("zever dev -addr 127.0.0.1:8090")+dim(" -grpc-addr 127.0.0.1:9090"))

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

	// Boot is check → generate → auto-migrate → route table: a fresh clone
	// converges before the first server starts. It runs before the watcher is
	// established so the files a generate writes cannot schedule their own
	// rebuild. Every step is best-effort — a failing schema or migration
	// prints and the loop keeps watching, so the next save can fix it
	// without the developer losing a running server.
	devPrepare(project, out)

	watcher, err := devNewWatcher(project)
	if err != nil {
		return err
	}

	defer func() { _ = watcher.Close() }()

	// The child deliberately does not inherit ctx: cancelling a
	// CommandContext child kills it outright, and dev's whole contract is that
	// a shutdown goes through the graceful SIGTERM path in devChild.stop, which
	// the deferred stop below and every restart both run.
	child, err := devStartChild(cfg.ServerEntry, cfg.Args, out)
	if err != nil {
		return err
	}

	defer func() { child.stop(cfg.StopGrace) }()

	_, _ = fmt.Fprintf(out, "%s: %s %s\n", cyan(devTag), dim("server at"), cyan(devServerURL(args)))

	_, _ = fmt.Fprintf(out, "%s: %s %s %s %s\n", cyan(devTag), dim("watching"), cyan(cfg.SchemaDir), dim("and"), cyan(cfg.ServerEntry)+dim(" for changes (Ctrl-C to stop)"))

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
			child = devRebuildWithGrace(project, args, out, child, cfg.StopGrace)
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

// devPrepare runs the one-time boot sequence before the first server start:
// compile-check the schemas, regenerate with the default pipeline, auto-
// migrate a fresh database, and print the route table. Every step is
// best-effort — a failure prints and never blocks the server from starting.
func devPrepare(project ProjectConfig, out io.Writer) {
	if err := devCompileFunc(project.SchemaDir, out); err != nil {
		_, _ = fmt.Fprintf(out, "%s: initial compile %s %s (see diagnostics above); starting the server anyway\n", cyan(devTag), yellow("FAILED"), failMark())
	}

	if err := devGenerateFunc(project, out); err != nil {
		_, _ = fmt.Fprintf(out, "%s: generate %s %s %v; server runs previous generated code\n", cyan(devTag), yellow("FAILED"), failMark(), red(err.Error()))
	}

	devMigrateFunc(project, out)
	devRoutesFunc(project.SchemaDir, out)
}

// devGenerate regenerates the project with the default backend pipeline (the
// bare `zever generate` run a fresh clone performs), writing into the
// project directory dev was started from, then tidies go.mod: the generate
// pulls new zever packages into the import graph (serve, the generated
// modules' deps) that the scaffold's go.mod does not know yet.
func devGenerate(project ProjectConfig, out io.Writer) error {
	modulePath, err := goModulePath()
	if err != nil {
		return fmt.Errorf("%s: %w", devTag, err)
	}

	if err := devGenerateFull(GenerateFullConfig{
		SchemaDir:   project.SchemaDir,
		ServerEntry: project.ServerEntry,
		ModulePath:  modulePath,
		Stdout:      out,
	}); err != nil {
		return err
	}

	return devTidy(out)
}

// devTidy runs `go mod tidy` in the working directory so the just-written
// imports resolve. Best-effort like every dev boot step: a tidy that fails
// (an offline machine, a hand-pinned module) prints and the server still
// starts on the previous generated code.
func devTidy(out io.Writer) error {
	cmd := exec.CommandContext(context.Background(), "go", "mod", "tidy")
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		_, _ = fmt.Fprintf(out, "%s: go mod tidy %s %s; run it by hand if the server fails to build\n", cyan(devTag), yellow("FAILED"), failMark())
	}

	return nil
}

// devAutoMigrate applies the schema to the dev database, but only where
// that is unambiguously safe: a sqlite database (a local file, the
// zero-infrastructure default) or an explicit dev ZEVER_ENV. Anything else —
// a postgres DSN from the environment, say — is migrated only after an
// interactive yes; headless, it prints the manual command instead of
// prompting, so a CI run never blocks on a TTY.
func devAutoMigrate(project ProjectConfig, out io.Writer) {
	adapter, dsn := devResolveDB()
	if adapter == "" {
		_, _ = fmt.Fprintf(out, "%s: auto-migrate skipped (no database configured)\n", cyan(devTag))

		return
	}

	if adapter != "sqlite" && !crypto.IsDevEnv(os.Getenv("ZEVER_ENV")) {
		_, _ = fmt.Fprintf(out, "%s: auto-migrate skipped (adapter %q): run 'zever db migrate' to apply\n", cyan(devTag), adapter)

		if isInteractiveTerminal() {
			run, err := promptConfirmForDev("apply migrations to this database now?", "y")
			if err == nil && run {
				devMigrateTarget(adapter, dsn, project, out)
			}
		}

		return
	}

	devMigrateTarget(adapter, dsn, project, out)
}

// devResolveDB returns the effective database target the same way
// `zever db migrate` does: the env-derived adapter plus that adapter's DSN
// (postgres) or file path (sqlite and friends), defaulting a sqlite path to
// the scaffold's data/app.db. Best-effort: an unresolvable environment
// yields empty strings, and auto-migrate stays silent rather than guessing.
func devResolveDB() (adapter, dsn string) {
	env, err := config.FromEnv()
	if err != nil {
		return "", ""
	}

	resolved, err := config.Resolve(env)
	if err != nil || resolved == nil {
		return "", ""
	}

	adapter = strings.TrimSpace(resolved.DB.Adapter)

	switch adapter {
	case "postgres":
		dsn = strings.TrimSpace(resolved.DB.Options.DSN)
	default:
		dsn = strings.TrimSpace(resolved.DB.Options.Path)
	}

	if adapter == "sqlite" && dsn == "" {
		dsn = DefaultDevDBPath
	}

	return adapter, dsn
}

// devMigrateTarget compiles the schema with the atlas backend and applies
// the diff to the named database. A broken schema or a failed apply prints
// and returns: auto-migrate is a convenience, never a boot blocker.
func devMigrateTarget(adapter, dsn string, project ProjectConfig, out io.Writer) {
	files, err := collectZenFiles(devTag, project.SchemaDir)
	if err != nil {
		_, _ = fmt.Fprintf(out, "%s: auto-migrate %s %v\n", cyan(devTag), yellow("FAILED"), err)

		return
	}

	if len(files) == 0 {
		_, _ = fmt.Fprintf(out, "%s: auto-migrate skipped (no .zen files under %s)\n", cyan(devTag), cyan(project.SchemaDir))

		return
	}

	result, diags := compile.Compile(files, atlas.New())
	if diags.HasErrors() {
		printDiagnostics(diags)

		_, _ = fmt.Fprintf(out, "%s: auto-migrate %s (schema does not compile; see diagnostics above)\n", cyan(devTag), yellow("FAILED"))

		return
	}

	if dir := filepath.Dir(dsn); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			_, _ = fmt.Fprintf(out, "%s: auto-migrate %s %v\n", cyan(devTag), yellow("FAILED"), err)

			return
		}
	}

	if err := applyDiff(adapter, dsn, result.Schema, migrate.PlanOptions{DetectTypeChanges: true}, false); err != nil {
		_, _ = fmt.Fprintf(out, "%s: auto-migrate %s %s %v\n", cyan(devTag), yellow("FAILED"), failMark(), red(err.Error()))
	}
}

// devPrintRoutes resolves the schema once and prints the HTTP route table —
// the same binding list `zever routes` shows — so a developer staring at a
// fresh dev server knows what to call.
func devPrintRoutes(schemaDir string, out io.Writer) {
	discovered, err := collectZenFiles(devTag, schemaDir)
	if err != nil || len(discovered) == 0 {
		return
	}

	files := make([]string, 0, len(discovered))
	for f := range discovered {
		files = append(files, f)
	}

	sort.Strings(files)

	if err := runRoutesWith(RoutesConfig{Files: files, Out: out}); err != nil {
		_, _ = fmt.Fprintf(out, "%s: route table %s %v\n", cyan(devTag), yellow("FAILED"), err)
	}
}

// devAddr extracts dev's -addr listen-address flag from args, in both
// "-addr 127.0.0.1:8081" and "-addr=127.0.0.1:8081" forms. A dangling flag
// with no value yields nothing: dev never invents an address, and the
// child's own flag parsing gets the argument to reject. -grpc-addr is
// deliberately not extracted: it rides along in the passthrough args only.
func devAddr(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		name, value, hasValue := strings.Cut(arg, "=")
		if name != "-addr" && name != "--addr" {
			continue
		}

		if !hasValue {
			if i+1 >= len(args) {
				continue
			}

			i++
			value = args[i]
		}

		return value
	}

	return ""
}

// devAddrPort extracts the listen port from an -addr value (":8080",
// "127.0.0.1:8080" or bare "8080"), reporting ok=false for anything else.
func devAddrPort(addr string) (port string, ok bool) {
	if addr == "" {
		return "", false
	}

	if _, port, err := net.SplitHostPort(addr); err == nil {
		if port == "" {
			return "", false
		}

		return port, true
	}

	if _, err := strconv.Atoi(addr); err == nil {
		return addr, true
	}

	return "", false
}

// devChildEnv builds the environment overrides for the server child. The
// dev boot marker is unconditional: serve refuses the deterministic dev
// crypto key without it. PORT mirrors -addr when the address carries an
// extractable port, as a belt-and-braces for entrypoints that read the
// environment instead of parsing flags; the generated entrypoint's own
// -addr parsing wins over it. GRPC_ADDR is deliberately never set: the
// generated entrypoint already gates -grpc-addr on auth+permission being
// wired (its grpcWired check), and an env-set GRPC_ADDR would bypass that
// gate straight into serve's fail-closed setupGRPC.
func devChildEnv(args []string) []string {
	env := []string{devEnvKey}

	addr := devAddr(args)

	if port, ok := devAddrPort(addr); ok {
		env = append(env, "PORT="+port)
	}

	return env
}

// devServerURL returns the URL line dev prints once the child is up: the
// -addr port when given, else PORT, else serve's default. It is a hint for
// the developer, not a claim about the bound address.
func devServerURL(args []string) string {
	addr := devAddr(args)

	if port, ok := devAddrPort(addr); ok {
		return "http://localhost:" + port
	}

	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return "http://localhost:" + port
	}

	return "http://localhost:" + serve.DefaultPort
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

	// The listen-address flags ride along in args untouched — the generated
	// entrypoint parses -addr/-grpc-addr itself — while devChildEnv layers
	// the dev boot marker and PORT override over the child environment.
	cmd, err := startLaunch(context.Background(), entryDir, args, devChildEnv(args)...)
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
