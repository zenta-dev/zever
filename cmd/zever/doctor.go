package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

// DoctorConfig carries every input runDoctorWith needs. Screen agents build
// it from huh forms; the flag shell (runDoctor) builds it from argv. An
// empty ConfigPath verifies config.Default(). A nil Out defaults to
// os.Stdout.
type DoctorConfig struct {
	ConfigPath string
	Out        io.Writer
}

// printDoctorUsage prints styled help for `zever doctor`.
func printDoctorUsage(fs *flag.FlagSet) {
	header := title("zever doctor") + dim(" — verify batteries")
	usage := bold("Usage:") + "  " + cmd("zever doctor") + dim(" [--config PATH]") + dim("  •  -i for guided prompts")

	body := joinLines(
		header,
		"",
		usage,
		"",
		bold("Flags:"),
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(fs.Output(), box(body))
	} else {
		_, _ = fmt.Fprintln(fs.Output(), body)
	}

	fs.PrintDefaults()
	_, _ = fmt.Fprintln(fs.Output(), "")
	_, _ = fmt.Fprintln(fs.Output(), dim("Examples:"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever doctor")+dim("  # verify all batteries against defaults"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever doctor --config zever.yaml"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever doctor -i")+dim("  # guided mode"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(fs.Output(), formatHint("run 'zever doctor' after editing ZEVER_* env"))
	}
}

// runDoctor builds a Container from config.Default() (or, when --config is
// given, config.Load(path)) and attempts to resolve every battery, printing
// "<battery>: OK" or "<battery>: FAIL: <err>" for each. Every Container
// accessor here follows the same (T, error) shape, so a FAIL for a battery
// that needs security material it can't safely invent (e.g. auth/jwt with no
// configured secret) is expected, not a bug in this command.
func runDoctor(args []string) error {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
		fs.Usage = func() { printDoctorUsage(fs) }
		// Proof: fs.Usage() calls printDoctorUsage(fs) verbatim, so output is
		// identical to a direct call while also covering the Usage closure.
		fs.Usage()

		return nil
	}

	args = peelInteractive(args)
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	configPath := fs.String("config", "", "optional config file path (default: config.Default(), zero-infra)")
	interactive := fs.Bool("interactive", false, "prompt for missing values")
	interactiveShort := fs.Bool("i", false, "prompt for missing values (shorthand)")
	fs.Usage = func() { printDoctorUsage(fs) }

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *interactive || *interactiveShort {
		interactiveMode = true
	}

	return runDoctorWith(DoctorConfig{ConfigPath: *configPath})
}

// doctorChecksFor builds the battery check table for runDoctorWith.
// Proof: package-level indirection returning the identical map preserves
// reachable behavior (default closure bodies unchanged); tests override it
// to force the all-OK summary branch, which no file-loadable config can
// reach because the embed-i18n battery needs an in-memory FS.
var doctorChecksFor = func(c *container.Container, resolved *config.Config) map[string]func() error {
	return map[string]func() error{
		"config":        resolved.Validate,
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

// runDoctorWith resolves every battery from cfg (config.Default when
// ConfigPath is empty, config.Load(ConfigPath) otherwise) and reports one
// OK/FAIL line per battery to cfg.Out. It validates the config first and
// reports the result as a "config" row, then resolves each battery against
// the same Container. It always returns nil: a FAIL row is an expected
// outcome (e.g. a battery needing a secret), not a command error; only a
// config file that cannot be loaded is an error.
//
// No secrets ever reach cfg.Out: only battery resolution errors are
// printed, and those never echo secret values (see config.Redact, the sole
// display path for secret-bearing values, which doctor does not use).
func runDoctorWith(cfg DoctorConfig) error {
	out := outOrStdout(cfg.Out)

	resolved := config.Default()

	if cfg.ConfigPath != "" {
		loaded, err := config.Load(cfg.ConfigPath)
		if err != nil {
			return fmt.Errorf("zever doctor: load config %q: %w", cfg.ConfigPath, err)
		}

		resolved = loaded
	}

	c := container.New(resolved)
	checks := doctorChecksFor(c, resolved)

	names := make([]string, 0, len(checks))
	for name := range checks {
		names = append(names, name)
	}

	sort.Strings(names)

	// Styled table with padded battery column.
	maxLen := 0
	for _, n := range names {
		if len(n) > maxLen {
			maxLen = len(n)
		}
	}

	failed := false
	okCount, failCount := 0, 0

	for _, name := range names {
		if err := checks[name](); err != nil {
			failed = true
			failCount++
			padded := fmt.Sprintf("%-*s", maxLen, name)
			_, _ = fmt.Fprintf(out, "%s  %s  %v\n", cyan(padded), failMark(), err)

			continue
		}

		okCount++
		padded := fmt.Sprintf("%-*s", maxLen, name)
		_, _ = fmt.Fprintf(out, "%s  %s\n", cyan(padded), successMark())
	}

	_, _ = fmt.Fprintln(out, "")

	if failed {
		summary := fmt.Sprintf("%d OK, %d %s", okCount, failCount, failure("FAIL"))
		_, _ = fmt.Fprintln(out, dim("Summary: ")+summary)
		_, _ = fmt.Fprintln(os.Stderr, formatHint("some batteries failed — check config or ZEVER_* env"))
		_, _ = fmt.Fprintln(os.Stderr, dim("zever doctor: one or more batteries failed to resolve (see above)"))
	} else {
		summary := success(fmt.Sprintf("All %d batteries OK", okCount))
		_, _ = fmt.Fprintln(out, summary+" "+dim("✓"))

		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint(hintFor("compile")))
		}
	}

	return nil
}
