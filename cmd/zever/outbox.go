package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/outbox"
)

// errOutboxUnknownSubcommand is returned when `zever outbox` is given no
// subcommand at all.
var errOutboxUnknownSubcommand = errors.New("zever outbox: missing subcommand (want: status, dlq, purge)")

// allOutbox and allOutboxDLQ back did-you-mean hints and usage text.
var (
	allOutbox    = []string{"status", "dlq", "purge"}
	allOutboxDLQ = []string{"list", "requeue", "purge"}
)

// runOutbox dispatches the two-word `zever outbox <subcommand>` family.
func runOutbox(args []string) error {
	args = peelInteractive(args)

	if len(args) == 0 {
		printOutboxUsage()

		return errOutboxUnknownSubcommand
	}

	sub, rest := args[0], args[1:]

	switch sub {
	case "status":
		return runOutboxStatus(rest)
	case "dlq":
		return runOutboxDLQ(rest)
	case "purge":
		return runOutboxPurge(rest)
	case "-h", "--help", "help":
		printOutboxUsage()

		return nil
	default:
		printOutboxUsage()

		if shouldShowHint() {
			if s := closest(sub, allOutbox); s != "" {
				_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
			}
		}

		return fmt.Errorf("%w: %q", ErrUnknownSubcommand, sub)
	}
}

// runOutboxDLQ dispatches the three-word `zever outbox dlq <subcommand>`
// family.
func runOutboxDLQ(args []string) error {
	if len(args) == 0 {
		printOutboxUsage()

		return errOutboxUnknownSubcommand
	}

	sub, rest := args[0], args[1:]

	switch sub {
	case "list":
		return runOutboxDLQList(rest)
	case "requeue":
		return runOutboxDLQRequeue(rest)
	case "purge":
		return runOutboxDLQPurge(rest)
	case "-h", "--help", "help":
		printOutboxUsage()

		return nil
	default:
		printOutboxUsage()

		if shouldShowHint() {
			if s := closest(sub, allOutboxDLQ); s != "" {
				_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
			}
		}

		return fmt.Errorf("%w: %q", ErrUnknownSubcommand, sub)
	}
}

// outboxFlags holds the shared connection flags every outbox subcommand
// accepts.
type outboxFlags struct {
	dsn    *string
	table  *string
	config *string
}

// newOutboxFlagSet builds the shared outbox connection flags for name.
func newOutboxFlagSet(name string) (*flag.FlagSet, outboxFlags) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	f := outboxFlags{
		dsn:    fs.String("dsn", "", "postgres DSN or sqlite path; default: zever.yaml outbox.dsn"),
		table:  fs.String("table", "", `outbox table name; default: zever.yaml outbox.table or "outbox"`),
		config: fs.String("config", "", "config file path (default: discover zever.yaml/.yml/.json + env)"),
	}

	return fs, f
}

// outboxTarget is the resolved connection target for one outbox command.
type outboxTarget struct {
	DSN   string
	Table string
}

// resolveOutboxTarget loads the outbox section from configPath (empty means
// discover zever.yaml/.yml/.json in the working directory plus env overrides)
// and applies the --dsn/--table flag overrides.
func resolveOutboxTarget(configPath, dsn, table string) (outboxTarget, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return outboxTarget{}, fmt.Errorf("zever outbox: load config %q: %w", configPath, err)
	}

	opts := cfg.Outbox.Options
	if dsn != "" {
		opts.DSN = dsn
	}

	if table != "" {
		opts.Table = table
	}

	return outboxTarget{DSN: opts.DSN, Table: opts.Table}, nil
}

// openOutboxStore opens a store at target; adapters.go registers the db
// factory at startup.
func openOutboxStore(target outboxTarget) (outbox.Store, error) {
	store, err := outbox.Open(outbox.DB, outbox.Options{DSN: target.DSN, Table: target.Table})
	if err != nil {
		return nil, fmt.Errorf("zever outbox: open db: %w", err)
	}

	return store, nil
}

// outboxAdminFor asserts the admin surface on an opened store.
func outboxAdminFor(store outbox.Store) (outbox.Admin, error) {
	admin, ok := store.(outbox.Admin)
	if !ok {
		return nil, fmt.Errorf("zever outbox: adapter %q does not support admin operations", store.Name())
	}

	return admin, nil
}

// outboxStatusData is the JSON shape of `zever outbox status`.
type outboxStatusData struct {
	Pending   int64  `json:"pending"`
	Processed int64  `json:"processed"`
	Failed    int64  `json:"failed"`
	LastError string `json:"last_error,omitempty"`
	Stalled   bool   `json:"stalled"`
}

// outboxRequeueData is the JSON shape of `zever outbox dlq requeue`.
type outboxRequeueData struct {
	ID  string `json:"id,omitempty"`
	All bool   `json:"all"`
}

// outboxDeleteData is the JSON shape of `zever outbox dlq purge`.
type outboxDeleteData struct {
	ID string `json:"id"`
}

// outboxPurgeData is the JSON shape of `zever outbox purge`.
type outboxPurgeData struct {
	Deleted int64 `json:"deleted"`
}

// runOutboxStatus prints the relay counters for the resolved outbox table.
func runOutboxStatus(args []string) error {
	if hasHelpFlag(args) {
		printOutboxUsage()

		return nil
	}

	fs, flags := newOutboxFlagSet("outbox status")
	fs.Usage = func() { printOutboxUsage() }

	if _, err := flexibleParse(fs, args); err != nil {
		return err
	}

	target, err := resolveOutboxTarget(*flags.config, *flags.dsn, *flags.table)
	if err != nil {
		return err
	}

	store, err := openOutboxStore(target)
	if err != nil {
		return err
	}

	defer func() { _ = store.Close() }()

	st := store.Status()

	if jsonMode {
		emitSuccess("outbox status", outboxStatusData{
			Pending:   st.Pending,
			Processed: st.Processed,
			Failed:    st.Failed,
			LastError: st.LastError,
			Stalled:   st.Stalled,
		})

		return nil
	}

	printOutboxStatus(target, st)

	return nil
}

// runOutboxDLQList lists failed messages (the DLQ), oldest first.
func runOutboxDLQList(args []string) error {
	if hasHelpFlag(args) {
		printOutboxUsage()

		return nil
	}

	fs, flags := newOutboxFlagSet("outbox dlq list")
	limit := fs.Int("limit", 0, "maximum messages to return (0 means the adapter default)")
	fs.Usage = func() { printOutboxUsage() }

	if _, err := flexibleParse(fs, args); err != nil {
		return err
	}

	target, err := resolveOutboxTarget(*flags.config, *flags.dsn, *flags.table)
	if err != nil {
		return err
	}

	store, err := openOutboxStore(target)
	if err != nil {
		return err
	}

	defer func() { _ = store.Close() }()

	admin, err := outboxAdminFor(store)
	if err != nil {
		return err
	}

	msgs, err := admin.List(context.Background(), "", *limit)
	if err != nil {
		return fmt.Errorf("zever outbox dlq list: %w", err)
	}

	if jsonMode {
		emitSuccess("outbox dlq list", msgs)

		return nil
	}

	printOutboxMessages(msgs)

	return nil
}

// runOutboxDLQRequeue moves failed messages back to pending.
func runOutboxDLQRequeue(args []string) error {
	if hasHelpFlag(args) {
		printOutboxUsage()

		return nil
	}

	fs, flags := newOutboxFlagSet("outbox dlq requeue")
	id := fs.String("id", "", "requeue only the failed message with this id")
	all := fs.Bool("all", false, "requeue every failed message")
	fs.Usage = func() { printOutboxUsage() }

	if _, err := flexibleParse(fs, args); err != nil {
		return err
	}

	if *all == (*id != "") {
		return errors.New("zever outbox dlq requeue: specify exactly one of --id or --all")
	}

	target, err := resolveOutboxTarget(*flags.config, *flags.dsn, *flags.table)
	if err != nil {
		return err
	}

	store, err := openOutboxStore(target)
	if err != nil {
		return err
	}

	defer func() { _ = store.Close() }()

	admin, err := outboxAdminFor(store)
	if err != nil {
		return err
	}

	if err := admin.Requeue(context.Background(), *id); err != nil {
		return fmt.Errorf("zever outbox dlq requeue: %w", err)
	}

	if jsonMode {
		emitSuccess("outbox dlq requeue", outboxRequeueData{ID: *id, All: *all})

		return nil
	}

	if *all {
		_, _ = fmt.Fprintln(os.Stdout, successMark()+" requeued failed message(s)")
	} else {
		_, _ = fmt.Fprintf(os.Stdout, "%s requeued %s\n", successMark(), cyan(*id))
	}

	return nil
}

// runOutboxDLQPurge deletes one failed message by id.
func runOutboxDLQPurge(args []string) error {
	if hasHelpFlag(args) {
		printOutboxUsage()

		return nil
	}

	fs, flags := newOutboxFlagSet("outbox dlq purge")
	id := fs.String("id", "", "delete the DLQ message with this id")
	fs.Usage = func() { printOutboxUsage() }

	if _, err := flexibleParse(fs, args); err != nil {
		return err
	}

	if strings.TrimSpace(*id) == "" {
		return errors.New("zever outbox dlq purge: --id is required")
	}

	target, err := resolveOutboxTarget(*flags.config, *flags.dsn, *flags.table)
	if err != nil {
		return err
	}

	store, err := openOutboxStore(target)
	if err != nil {
		return err
	}

	defer func() { _ = store.Close() }()

	deleter, ok := store.(outbox.Deleter)
	if !ok {
		return fmt.Errorf("zever outbox: adapter %q does not support message deletion", store.Name())
	}

	if err := deleter.Delete(context.Background(), *id); err != nil {
		return fmt.Errorf("zever outbox dlq purge: %w", err)
	}

	if jsonMode {
		emitSuccess("outbox dlq purge", outboxDeleteData{ID: *id})

		return nil
	}

	_, _ = fmt.Fprintf(os.Stdout, "%s deleted %s\n", successMark(), cyan(*id))

	return nil
}

// runOutboxPurge deletes processed messages older than --before.
func runOutboxPurge(args []string) error {
	if hasHelpFlag(args) {
		printOutboxUsage()

		return nil
	}

	fs, flags := newOutboxFlagSet("outbox purge")
	before := fs.String("before", "", "purge processed messages older than this duration (for example 168h)")
	fs.Usage = func() { printOutboxUsage() }

	if _, err := flexibleParse(fs, args); err != nil {
		return err
	}

	if strings.TrimSpace(*before) == "" {
		return errors.New("zever outbox purge: --before is required (for example --before 168h)")
	}

	age, err := time.ParseDuration(*before)
	if err != nil {
		return fmt.Errorf("zever outbox purge: invalid --before %q: %w", *before, err)
	}

	if age < 0 {
		return errors.New("zever outbox purge: --before must not be negative")
	}

	target, err := resolveOutboxTarget(*flags.config, *flags.dsn, *flags.table)
	if err != nil {
		return err
	}

	store, err := openOutboxStore(target)
	if err != nil {
		return err
	}

	defer func() { _ = store.Close() }()

	admin, err := outboxAdminFor(store)
	if err != nil {
		return err
	}

	n, err := admin.Purge(context.Background(), time.Now().UTC().Add(-age))
	if err != nil {
		return fmt.Errorf("zever outbox purge: %w", err)
	}

	if jsonMode {
		emitSuccess("outbox purge", outboxPurgeData{Deleted: n})

		return nil
	}

	_, _ = fmt.Fprintf(os.Stdout, "%s purged %d processed message(s)\n", successMark(), n)

	return nil
}

// printOutboxStatus writes the human-readable status block to stdout.
func printOutboxStatus(target outboxTarget, st outbox.Status) {
	table := target.Table
	if table == "" {
		table = outbox.DefaultTable
	}

	_, _ = fmt.Fprintf(os.Stdout, "%s %s\n", title("outbox status"), dim("table="+table))
	_, _ = fmt.Fprintf(os.Stdout, "  %-10s %d\n", "pending", st.Pending)
	_, _ = fmt.Fprintf(os.Stdout, "  %-10s %d\n", "processed", st.Processed)
	_, _ = fmt.Fprintf(os.Stdout, "  %-10s %d\n", "failed", st.Failed)

	if st.LastError != "" {
		_, _ = fmt.Fprintf(os.Stdout, "  %-10s %s\n", "last_error", st.LastError)
	}

	_, _ = fmt.Fprintf(os.Stdout, "  %-10s %t\n", "stalled", st.Stalled)
}

// printOutboxMessages writes one line per message to stdout.
func printOutboxMessages(msgs []outbox.Message) {
	if len(msgs) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, dim("no messages"))

		return
	}

	for _, m := range msgs {
		_, _ = fmt.Fprintf(os.Stdout, "%s  %s  attempts=%d  created=%s\n",
			cyan(m.ID), m.Topic, m.Attempts, m.CreatedAt.UTC().Format(time.RFC3339))
	}
}

// printOutboxUsage prints styled help for the `zever outbox` family.
func printOutboxUsage() {
	header := title("zever outbox") + dim(" — outbox DLQ operations")
	usage := bold("Usage:") + "  " + cmd("zever outbox") + dim(" <subcommand> [flags]")
	line := func(name, desc string) string {
		return "  " + cmd(fmt.Sprintf("%-16s", name)) + dim(desc)
	}

	body := joinLines(
		header,
		"",
		usage,
		"",
		bold("Subcommands:"),
		line("status", "Show pending/processed/failed counters"),
		line("dlq list", "List failed messages (DLQ), oldest first"),
		line("dlq requeue", "Requeue failed messages (--id X | --all)"),
		line("dlq purge", "Delete one failed message (--id X)"),
		line("purge", "Delete processed messages older than --before"),
		"",
		bold("Connection flags:"),
		line("--dsn", "postgres DSN or sqlite path (default: zever.yaml)"),
		line("--table", `outbox table name (default: "outbox")`),
		line("--config", "config file path (default: discover zever.yaml)"),
		"",
		dim("Run '")+cmd("zever outbox <subcommand> -h")+dim("' for flags  •  ")+hint("tip: ")+dim("use --json for machine-readable output"),
	)

	if colorEnabled {
		_, _ = fmt.Fprintln(os.Stderr, box(body))
	} else {
		_, _ = fmt.Fprintln(os.Stderr, body)
	}
}

// newOutboxCmd returns the `outbox` parent with status/dlq/purge subcommands.
// Each subcommand wraps runOutbox with fixed sub-args; the stdlib flag parsing
// inside the runOutbox* handlers stays authoritative, so every subcommand sets
// DisableFlagParsing and passes raw args through. The bare parent delegates to
// runOutbox so `zever outbox` keeps its missing-subcommand usage + error
// contract.
func newOutboxCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "outbox",
		Short: "Inspect and repair the outbox dead-letter queue",
		Long: `Inspect and repair the transactional outbox: show relay
counters, list failed messages, requeue them for redelivery, and
purge processed history.`,
		Example: `  zever outbox status --dsn data/app.db
  zever outbox dlq list --limit 20
  zever outbox dlq requeue --all
  zever outbox purge --before 168h`,
		Args: cobra.ArbitraryArgs,
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

			return runOutbox(args)
		},
	}

	sub := func(name, short, long, example string) *cobra.Command {
		return &cobra.Command{
			Use:                name,
			Short:              short,
			Long:               long,
			Example:            example,
			Args:               cobra.ArbitraryArgs,
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if versionRequested(cmd) {
					printCLIVersion()

					return nil
				}

				return runOutbox(append([]string{name}, args...))
			},
		}
	}

	parent.AddCommand(
		sub("status", "Show relay counters",
			"Show the outbox relay's pending, processed, and failed counters.",
			"  zever outbox status --dsn data/app.db"),
		sub("dlq", "Inspect and repair failed messages",
			"List, requeue, or delete messages that exhausted their retry budget.",
			"  zever outbox dlq list --limit 20"),
		sub("purge", "Delete processed history",
			"Delete processed messages older than the given duration.",
			"  zever outbox purge --before 168h"),
	)

	return parent
}
