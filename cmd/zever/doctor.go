package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

// DoctorConfig carries every input runDoctorWith needs. Screen agents build
// it from huh forms; the flag shell (runDoctor) builds it from argv. An
// empty ConfigPath runs config.Load(""), which discovers
// zever.yaml/.yml/.json in the working directory and applies env overrides.
// A nil Out defaults to os.Stdout.
type DoctorConfig struct {
	ConfigPath string
	Out        io.Writer
	// Strict is retained for flag compatibility: since P3-3 any failing
	// battery returns a non-nil error (exit 1) with or without it, so
	// `zever doctor` gates CI/pre-deploy pipelines by default.
	Strict bool
	// JSON emits a machine-readable envelope instead of the human table.
	JSON bool
}

// printDoctorUsage prints styled help for `zever doctor`.
func printDoctorUsage(fs *flag.FlagSet) {
	header := title("zever doctor") + dim(" — verify project batteries")
	usage := bold("Usage:") + "  " + cmd("zever doctor") + dim(" [--config PATH] [--strict]") + dim("  •  -i for guided prompts")

	body := joinLines(
		header,
		"",
		usage,
		"",
		"Only the services enabled in the project config are checked: the",
		"top-level batteries in zever.yaml/.yml/.json (same discovery as",
		"config.Load and serve). In a project with a server entrypoint the",
		"checks run inside the built server ("+cmd("go run <entry> --doctor")+")",
		"against its real container and registrations; otherwise they run in",
		"the CLI with a note. Without a project config every battery is",
		"checked, also with a note.",
		"",
		"Shared-pool MaxConns mismatches (a borrower capped differently",
		"from the first opener) print as pool warnings with the",
		"dedicated_pool fix hint; they never fail the command.",
		"",
		"Exit codes: 0 all OK, 1 failures, 2 flag misuse.",
		"",
		bold("Flags:"),
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(fs.Output(), box(body))
	} else {
		_, _ = fmt.Fprintln(fs.Output(), body)
	}

	printFlagDefaults(fs)
	_, _ = fmt.Fprintln(fs.Output(), "")
	_, _ = fmt.Fprintln(fs.Output(), dim("Examples:"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever doctor")+dim("  # verify enabled project batteries"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever doctor --config zever.yaml"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever doctor -i")+dim("  # guided mode"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(fs.Output(), formatHint("run 'zever doctor' after editing ZEVER_* env"))
	}
}

// newDoctorFlagSet declares the doctor flags once, shared by argv parsing and
// every help path so `zever help doctor` lists the real flags.
func newDoctorFlagSet() (fs *flag.FlagSet, configPath *string, strict *bool) {
	fs = flag.NewFlagSet("doctor", flag.ContinueOnError)
	configPath = fs.String("config", "", "optional config file path (default: discover zever.yaml/.yml/.json + env)")
	strict = fs.Bool("strict", false, "accepted for compatibility; failures now always exit non-zero")
	fs.Usage = func() { printDoctorUsage(fs) }

	return fs, configPath, strict
}

// runDoctor builds a Container from config.Load(path) (which discovers
// zever.yaml/.yml/.json and applies env when --config is absent) and checks
// only the project's enabled batteries (see doctorScope), printing
// "<battery>: OK" or "<battery>: FAIL: <err>" for each. Every Container
// accessor here follows the same (T, error) shape, so a FAIL for a battery
// that needs security material it can't safely invent (e.g. auth/jwt with no
// configured secret) is expected, not a bug in this command.
//
// Exit codes: nil (0) when every checked battery resolves, a non-nil error
// (exit 1) when any fails, and a flagUsageError (exit 2) on flag misuse.
func runDoctor(args []string) error {
	if hasHelpFlag(args) {
		fs, _, _ := newDoctorFlagSet()
		fs.Usage = func() { printDoctorUsage(fs) }
		// Proof: fs.Usage() calls printDoctorUsage(fs) verbatim, so output is
		// identical to a direct call while also covering the Usage closure.
		fs.Usage()

		return nil
	}

	args = peelInteractive(args)
	fs, configPath, strict := newDoctorFlagSet()

	if err := rejectUnknownFlags("doctor", args, []string{"config", "strict"}); err != nil {
		return &flagUsageError{err: err}
	}

	if err := fs.Parse(args); err != nil {
		return &flagUsageError{err: fmt.Errorf("zever doctor: %w", err)}
	}

	return runDoctorWith(DoctorConfig{ConfigPath: *configPath, Strict: *strict, JSON: jsonMode})
}

// doctorChecksFor builds the battery check table for runDoctorWith.
// Proof: package-level indirection returning the identical map preserves
// reachable behavior (default closure bodies unchanged); tests override it
// to force the all-OK summary branch, which no file-loadable config can
// reach because the embed-i18n battery needs an in-memory FS.
var doctorChecksFor = func(c *container.Container, resolved *config.Config) map[string]func() error {
	return map[string]func() error{
		"config":        resolved.Validate,
		"adapters":      c.ValidateAdapters,
		"ai":            func() error { _, err := c.AI(); return err },
		"analytics":     func() error { _, err := c.Analytics(); return err },
		"auth":          func() error { _, err := c.Auth(); return err },
		"billing":       func() error { _, err := c.Billing(); return err },
		"cache":         func() error { _, err := c.Cache(); return err },
		"crypto":        func() error { _, err := c.Crypto(); return err },
		"db":            func() error { _, err := c.DB(); return err },
		"document":      func() error { _, err := c.Document(); return err },
		"eventbus":      func() error { _, err := c.EventBus(); return err },
		"flag":          func() error { _, err := c.Flag(); return err },
		"geo":           func() error { _, err := c.Geo(); return err },
		"i18n":          func() error { _, err := c.I18n(); return err },
		"idempotency":   func() error { _, err := c.Idempotency(); return err },
		"job":           func() error { _, err := c.Job(); return err },
		"log":           func() error { _, err := c.Log(); return err },
		"mailer":        func() error { _, err := c.Mailer(); return err },
		"media":         func() error { _, err := c.Media(); return err },
		"notification":  func() error { _, err := c.Notification(); return err },
		"observability": func() error { _, err := c.Observability(); return err },
		"password":      func() error { _, err := c.Password(); return err },
		"payment":       func() error { _, err := c.Payment(); return err },
		"permission":    func() error { _, err := c.Permission(); return err },
		"queue":         func() error { _, err := c.Queue(); return err },
		"ratelimit":     func() error { _, err := c.RateLimit(); return err },
		"router":        func() error { _, err := c.Router(); return err },
		"scheduler":     func() error { _, err := c.Scheduler(); return err },
		"search":        func() error { _, err := c.Search(); return err },
		"session":       func() error { _, err := c.Session(); return err },
		"storage":       func() error { _, err := c.Storage(); return err },
		"tenant":        func() error { _, err := c.Tenant(); return err },
		"vectorstore":   func() error { _, err := c.VectorStore(); return err },
		"webhook":       func() error { _, err := c.Webhook(); return err },
		"workflow":      func() error { _, err := c.Workflow(); return err },
	}
}

// scopedValidateAdapters validates the resolved config against the
// process's registrations, skipping batteries the project does not enable:
// a slim `zever build --adapters=...` binary registers only its subset, so
// validating excluded batteries would fail a binary that is correct for its
// project. A nil enabled set (no project config found) keeps the strict
// all-battery check. Enabled-but-unregistered batteries still fail closed.
func scopedValidateAdapters(resolved *config.Config, enabled map[string]bool, registered func(service string) []string) error {
	if enabled == nil {
		return resolved.ValidateAdapters(registered)
	}

	keys := make([]string, 0, len(enabled))
	for k := range enabled {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return resolved.ValidateEnabledAdapters(keys, registered)
}

// doctorRow is one battery's check result, human and JSON alike.
type doctorRow struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// doctorScope is the resolved check plan: which batteries to check, where the
// checks run, and the note explaining the choice to the operator.
type doctorScope struct {
	// enabled lists the project's enabled batteries by name. Nil means no
	// project config was found, so every known battery is checked.
	enabled map[string]bool
	// source names where the checks run: "cli", or "server:<entry>".
	source string
	// serverEntry is the project server entrypoint to exec (--doctor mode),
	// or "" when no built server is available.
	serverEntry string
	// note explains the scope/source choice; empty when unremarkable.
	note string
}

// resolveDoctorScope locates the project the same way serve does:
// discoverServeConfig probes zever.yaml/.yml/.json in the working directory
// (first match wins), and serveYamlBatteries reads its top-level service
// keys — exactly the batteries `zever new` wrote, since the scaffold emits
// one stanza per enabled battery and nothing else. An explicit --config path
// pins the file instead of discovery. With no file at all every battery is
// checked and the note says so.
func resolveDoctorScope(configPath string) doctorScope {
	path := configPath
	if path == "" {
		path = discoverServeConfig()
	}

	if path == "" {
		return doctorScope{source: "cli", note: "no project config found (zever.yaml/.yml/.json) — checking all batteries"}
	}

	keys, err := serveYamlBatteries(path)
	if err != nil {
		return doctorScope{source: "cli", note: fmt.Sprintf("could not read batteries from %s — checking all batteries", path)}
	}

	enabled := make(map[string]bool, len(keys))
	for _, k := range keys {
		enabled[k] = true
	}

	scope := doctorScope{enabled: enabled, source: "cli"}

	entry := defaultServerEntry
	if pc, err := loadProjectConfig(); err == nil && strings.TrimSpace(pc.ServerEntry) != "" {
		entry = pc.ServerEntry
	}

	if fi, err := os.Stat(entry); err == nil && fi.IsDir() {
		scope.serverEntry = entry
	} else {
		scope.note = fmt.Sprintf("no built server entrypoint %q — checked via CLI registry", entry)
	}

	return scope
}

// localDoctorRows resolves the scope's enabled batteries against the CLI
// process's own container and registrations (a slim `zever build` binary
// registers only its adapter subset; the adapters meta-check is scoped to
// the enabled set so it stays green there). In deterministic name order.
// The "config" row always runs.
func localDoctorRows(c *container.Container, resolved *config.Config, enabled map[string]bool) []doctorRow {
	checks := doctorChecksFor(c, resolved)
	if _, ok := checks["adapters"]; ok {
		checks["adapters"] = func() error {
			return scopedValidateAdapters(resolved, enabled, container.RegisteredAdapters)
		}
	}

	names := make([]string, 0, len(checks))
	for name := range checks {
		if name == "config" || name == "adapters" || enabled == nil || enabled[name] {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	rows := make([]doctorRow, 0, len(names))
	for _, name := range names {
		if err := checks[name](); err != nil {
			rows = append(rows, doctorRow{Name: name, Error: err.Error()})

			continue
		}

		rows = append(rows, doctorRow{Name: name, OK: true})
	}

	return rows
}

// serverDoctorRows converts a built server's --doctor report into doctor
// rows: the config validation row always runs locally (the server only
// reports batteries), and server rows outside the enabled set are dropped.
func serverDoctorRows(resolved *config.Config, rep serverDoctorReport, enabled map[string]bool) []doctorRow {
	rows := make([]doctorRow, 0, len(rep.Batteries)+1)
	if err := resolved.Validate(); err != nil {
		rows = append(rows, doctorRow{Name: "config", Error: err.Error()})
	} else {
		rows = append(rows, doctorRow{Name: "config", OK: true})
	}

	names := make([]string, 0, len(rep.Batteries))
	byName := make(map[string]serverDoctorBattery, len(rep.Batteries))

	for _, b := range rep.Batteries {
		if _, seen := byName[b.Name]; seen {
			continue
		}

		byName[b.Name] = b

		if b.Name == "adapters" || enabled == nil || enabled[b.Name] {
			names = append(names, b.Name)
		}
	}

	sort.Strings(names)

	for _, name := range names {
		b := byName[name]
		rows = append(rows, doctorRow(b))
	}

	return rows
}

// shortDoctorErr keeps one line of err for the fallback note.
func shortDoctorErr(err error) string {
	msg := err.Error()
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}

	return msg
}

// runDoctorWith resolves the configured batteries and reports one OK/FAIL
// line per battery to cfg.Out. It validates the config first and reports the
// result as a "config" row, then resolves each enabled battery. A config
// that cannot be loaded is an error; any failing battery is also an error
// (exit 1), so `zever doctor` gates CI/pre-deploy pipelines by default.
//
// No secrets ever reach cfg.Out: only battery resolution errors are
// printed, and those never echo secret values (see config.Redact, the sole
// display path for secret-bearing values, which doctor does not use).
func runDoctorWith(cfg DoctorConfig) error {
	return runDoctorWithContext(context.Background(), cfg)
}

// runDoctorWithContext is runDoctorWith with a caller context, which governs
// the built-server `--doctor` exec (see runServerDoctor). Tests pass
// t.Context(); the flag shell has no caller context and uses Background.
func runDoctorWithContext(ctx context.Context, cfg DoctorConfig) error {
	out := outOrStdout(cfg.Out)

	resolved, err := config.Load(cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("zever doctor: load config %q: %w", cfg.ConfigPath, err)
	}

	c := container.New(resolved)
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = c.Close(closeCtx)
	}()

	scope := resolveDoctorScope(cfg.ConfigPath)

	var rows []doctorRow
	local := false
	if scope.serverEntry != "" {
		if rep, rerr := runServerDoctor(ctx, scope.serverEntry); rerr == nil {
			rows = serverDoctorRows(resolved, rep, scope.enabled)
			scope.source = "server:" + scope.serverEntry
			scope.note = fmt.Sprintf("checked via built server entrypoint %s --doctor (real container + registrations)", scope.serverEntry)
		} else {
			rows = localDoctorRows(c, resolved, scope.enabled)
			local = true
			scope.note = fmt.Sprintf("built server unavailable (%s) — checked via CLI registry", shortDoctorErr(rerr))
		}
	} else {
		rows = localDoctorRows(c, resolved, scope.enabled)
		local = true
	}

	// poolWarnings surfaces shared-pool MaxConns mismatches recorded while
	// resolving the batteries above (see container.PoolMismatches): a
	// borrower whose cap differs from the first opener's is silently
	// capped today, so doctor repeats the warning with the dedicated_pool
	// fix hint. Only the local path resolves through c; a built server
	// reports its own pools via its --doctor output.
	var poolWarnings []string
	if local {
		for _, m := range c.PoolMismatches() {
			poolWarnings = append(poolWarnings, m.String())
		}
	}

	failed := false
	okCount, failCount := 0, 0

	for _, row := range rows {
		if row.OK {
			okCount++
		} else {
			failed = true
			failCount++
		}
	}

	data := map[string]any{
		"batteries": rows,
		"ok":        okCount,
		"failed":    failCount,
		"source":    scope.source,
	}
	if scope.note != "" {
		data["note"] = scope.note
	}
	if len(poolWarnings) > 0 {
		data["pool_warnings"] = poolWarnings
	}

	if cfg.JSON {
		if failed {
			// A failing battery is a command failure in JSON mode: emit
			// one failure envelope (ok:false) and return its
			// already-emitted error so the process exits nonzero without
			// main writing a second envelope.
			msg := fmt.Errorf("zever doctor: %d of %d batteries failed", failCount, len(rows))

			return emitFailure("doctor", data, nil, msg)
		}

		emitSuccess("doctor", data)

		return nil
	}

	if scope.note != "" {
		_, _ = fmt.Fprintln(out, dim("note: ")+scope.note)
	}
	for _, w := range poolWarnings {
		_, _ = fmt.Fprintln(out, dim("note: ")+w)
	}

	// Styled table with padded battery column.
	maxLen := 0
	for _, row := range rows {
		if len(row.Name) > maxLen {
			maxLen = len(row.Name)
		}
	}

	for _, row := range rows {
		padded := fmt.Sprintf("%-*s", maxLen, row.Name)
		if row.OK {
			_, _ = fmt.Fprintf(out, "%s  %s\n", cyan(padded), successMark())

			continue
		}

		_, _ = fmt.Fprintf(out, "%s  %s  %s\n", cyan(padded), failMark(), row.Error)
	}

	_, _ = fmt.Fprintln(out, "")

	if failed {
		summary := fmt.Sprintf("%d OK, %d %s", okCount, failCount, failure("FAIL"))
		_, _ = fmt.Fprintln(out, dim("Summary: ")+summary)
		_, _ = fmt.Fprintln(os.Stderr, formatHint("some batteries failed — check config or ZEVER_* env"))
		_, _ = fmt.Fprintln(os.Stderr, dim("zever doctor: one or more batteries failed to resolve (see above)"))

		return fmt.Errorf("zever doctor: %d of %d batteries failed", failCount, len(rows))
	}

	summary := success(fmt.Sprintf("All %d batteries OK", okCount))
	_, _ = fmt.Fprintln(out, summary+" "+dim("✓"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(os.Stderr, formatHint(hintFor("compile")))
	}

	return nil
}
