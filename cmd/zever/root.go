package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// errFlagUsage is returned (wrapped) when Cobra flag parsing fails. main maps
// it to exit 2; every other error maps to exit 1.
var errFlagUsage = errors.New("zever: invalid flags")

// flagUsageError carries both the errFlagUsage sentinel (for the exit-2
// mapping in main) and the underlying pflag error (for the message).
// errors.Is matches either target via multi-Unwrap.
type flagUsageError struct {
	err error
}

// Error implements error.
func (e *flagUsageError) Error() string { return errFlagUsage.Error() + ": " + e.err.Error() }

// Unwrap returns both wrapped errors for errors.Is/As.
func (e *flagUsageError) Unwrap() []error { return []error{errFlagUsage, e.err} }

// errHelpShown short-circuits a pass-through RunE after printHelpIfRequested
// has printed help. cmd.Help() returns nil on success, which callers must
// not mistake for "no help requested, run the handler".
var errHelpShown = errors.New("zever: help shown")

// ErrUnknownSubcommand is returned when a command gets an unrecognized subcommand.
var ErrUnknownSubcommand = errors.New("zever: unknown subcommand")

// printHelpIfRequested implements -h/--help/help for pass-through commands
// (DisableFlagParsing leaves flag parsing to the legacy handlers, which
// historically mistook "-h" for input, e.g. `routes -h` tried to read a file
// named "-h"). It prints Cobra help and returns errHelpShown; nil means no
// help was requested and the handler should run.
func printHelpIfRequested(cmd *cobra.Command, args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			if err := cmd.Help(); err != nil {
				return err
			}

			return errHelpShown
		}
	}

	return nil
}

// quietMode holds whether -q/--quiet was requested. shouldShowHint (hint.go)
// reads it to suppress hint footers in scripting contexts.
var quietMode bool

// rootCmd is the Cobra root for the ported subcommands. Built by an explicit
// constructor call (no init wiring, per repo convention).
var rootCmd = newRootCmd()

// portedCommands lists the top-level subcommands handled by Cobra. main
// routes invocations whose first positional matches one of these to rootCmd
// and falls back to the legacy run() dispatch for everything else
// (unported commands, help aliases, unknown subcommands with our
// closest/formatHint did-you-mean style).
var portedCommands = []string{
	"new", "add", "generate", "extract",
	"compile", "check", "breaking", "fmt", "doctor", "config",
	"routes", "explain", "check-boundaries", "check:boundaries", "graph",
	"serve", "dev", "worker", "queue:work", "schedule", "schedule:run", "tinker",
	"db", "outbox", "completion", "docs", "uninstall", "upgrade", "build",
}

// portedSet is the lookup form of portedCommands.
var portedSet = func() map[string]bool {
	m := make(map[string]bool, len(portedCommands))
	for _, name := range portedCommands {
		m[name] = true
	}

	return m
}()

// knownDBSubcommands gates Cobra routing for `zever db`: only known
// sub-subcommands go through Cobra. Unknown ones stay on the legacy runDB
// path so the `did you mean` hint and `zever db: unknown subcommand` error
// keep their exact shape.
var knownDBSubcommands = map[string]bool{
	"migrate": true, "rollback": true, "seed": true,
}

// knownOutboxSubcommands gates Cobra routing for `zever outbox` the same way
// knownDBSubcommands does for `zever db`.
var knownOutboxSubcommands = map[string]bool{
	"status": true, "dlq": true, "purge": true,
}

// globalFlagToken reports whether tok is one of the boolean global flags that
// may precede the subcommand. Used only to locate the first positional for
// Cobra-vs-legacy routing; Cobra itself does the real parsing.
func globalFlagToken(tok string) bool {
	switch tok {
	case "-i", "--interactive", "-q", "--quiet", "--no-color", "--json", "-V", "--version":
		return true
	default:
		return false
	}
}

// useCobra reports whether args should be executed via rootCmd. It finds the
// first non-global-flag positional and checks it against portedSet. Cobra
// suggestions stay disabled (DisableSuggestions): unknown subcommands fall
// through to run(), which prints usage plus our own closest/formatHint
// `hint:` line for a consistent style.
func useCobra(args []string) bool {
	first := ""
	rest := []string{}

	// Cobra's hidden dynamic-completion commands, invoked by the scripts
	// `zever completion <shell>` generates.
	if len(args) > 0 && (args[0] == cobra.ShellCompRequestCmd || args[0] == cobra.ShellCompNoDescRequestCmd) {
		return true
	}

	for i, a := range args {
		if globalFlagToken(a) {
			continue
		}

		if len(a) > 0 && a[0] == '-' {
			// Unknown flag shape before any positional: let Cobra's
			// parser reject it so main maps it to exit 2 (flag misuse).
			return true
		}

		first = a
		rest = args[i+1:]

		break
	}

	if first == "" {
		return false
	}

	if !portedSet[first] {
		return false
	}

	if first == "db" {
		// Route only known db subcommands (or none/flag) via Cobra; anything
		// else keeps the legacy runDB unknown-subcommand behavior.
		for _, a := range rest {
			if globalFlagToken(a) {
				continue
			}

			if len(a) > 0 && a[0] == '-' {
				return true
			}

			return knownDBSubcommands[a]
		}

		return true
	}

	if first == "outbox" {
		// Mirror the db gate: unknown outbox subcommands keep the legacy
		// did-you-mean behavior.
		for _, a := range rest {
			if globalFlagToken(a) {
				continue
			}

			if len(a) > 0 && a[0] == '-' {
				return true
			}

			return knownOutboxSubcommands[a]
		}

		return true
	}

	return true
}

// versionRequested reports whether -V/--version was passed to cmd.
func versionRequested(cmd *cobra.Command) bool {
	if v, err := cmd.Flags().GetBool("V"); err == nil && v {
		return true
	}

	if v, err := cmd.Flags().GetBool("version"); err == nil && v {
		return true
	}

	return false
}

// printCLIVersion prints the CLI version to stdout (stdout stays clean:
// version output never goes to stderr).
func printCLIVersion() {
	_, _ = fmt.Fprintln(os.Stdout, "zever v"+buildVersion)
}

// newRootCmd builds the Cobra root: usage metadata, silenced usage/errors
// (runtime errors print via main, never with a usage dump), the CLI version,
// persistent global flags, and the ported command set.
func newRootCmd() *cobra.Command {
	var interactive, quiet, noColor, showV, showJSON bool

	root := &cobra.Command{
		Use:   "zever",
		Short: "Zever application toolkit",
		Long: `Zever application toolkit: scaffold, inspect, run, and migrate
zever applications built from .zen schemas.`,
		Version:            buildVersion,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableSuggestions: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if versionRequested(cmd) {
				printCLIVersion()

				return nil
			}

			if len(args) == 0 {
				// ONE help renderer: bare `zever` prints exactly what
				// `zever --help` prints (the Cobra tree with groups),
				// then exits 1 via errMissingSubcommand (main maps it
				// to exit 1; --help itself exits 0).
				if err := cmd.Help(); err != nil {
					return err
				}

				return errMissingSubcommand
			}

			// Positional under bare root (only reachable when useCobra
			// misroutes, e.g. direct Execute in tests): keep legacy shape.
			printUsage()

			if shouldShowHint() {
				// Derive from cmd's own tree (not the rootCmd global):
				// this closure runs inside newRootCmd, which initializes
				// that global — referencing it here would be an init cycle.
				if s := closest(args[0], topLevelNames(cmd.Root())); s != "" {
					_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
				}
			}

			return fmt.Errorf("%w: %q", ErrUnknownSubcommand, args[0])
		},
	}

	root.PersistentFlags().BoolVarP(&interactive, "interactive", "i", false, usageInteractive)
	root.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, usageQuiet)
	root.PersistentFlags().Bool("no-color", false, usageNoColor)
	root.PersistentFlags().BoolVar(&showJSON, "json", false, usageJSON)
	root.PersistentFlags().BoolVar(&showV, "V", false, usageV)

	root.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		if interactive || envInteractive() {
			interactiveMode = true
		}

		if noColor {
			colorEnabled = false
		}

		if cmd.Root().PersistentFlags().Changed("json") {
			jsonExplicit = true
			jsonMode = showJSON
		} else if envJSON() {
			jsonMode = true
		}

		quietMode = quiet
	}

	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &flagUsageError{err: err}
	})

	for _, c := range newScaffoldCmds() {
		root.AddCommand(c)
	}

	for _, c := range newRuntimeCmds() {
		root.AddCommand(c)
	}

	// Canonical space-form commands. The legacy colon spellings from
	// newRuntimeCmds stay registered but hidden: existing scripts keep
	// working, --help shows only the canonical form, and the suggestion
	// list (topLevelNames, from the Cobra tree) still offers both.
	for _, c := range root.Commands() {
		switch c.Name() {
		case "queue:work", "schedule:run":
			c.Hidden = true
		}
	}

	root.AddCommand(newWorkerCmd())
	root.AddCommand(newScheduleCmd())

	root.AddCommand(newDBCmd())

	root.AddCommand(newOutboxCmd())

	for _, c := range newInspectCmds() {
		root.AddCommand(c)
	}

	root.AddCommand(newCompletionCmd(root))
	root.AddCommand(newDocsCmd(root))
	root.AddCommand(newUninstallCmd())
	root.AddCommand(newUpgradeCmd())
	root.AddCommand(newBuildCmd())

	// Command groups in --help. Start holds the first commands a new
	// project touches; Build holds scaffolding/validation; Data holds the
	// database family; Run & ops holds the runtime and operations loop.
	// Anything unlisted (compile, breaking, routes, explain, graph,
	// config, check-boundaries, outbox, completion, docs, build,
	// uninstall, extract, plus the hidden legacy spellings) renders under
	// Cobra's default Additional Commands section.
	root.AddGroup(
		&cobra.Group{ID: "start", Title: "Start:"},
		&cobra.Group{ID: "build", Title: "Build:"},
		&cobra.Group{ID: "data", Title: "Data:"},
		&cobra.Group{ID: "run", Title: "Run & ops:"},
	)

	groupOf := map[string]string{
		"new":      "start",
		"dev":      "start",
		"add":      "build",
		"generate": "build",
		"fmt":      "build",
		"check":    "build",
		"db":       "data",
		"serve":    "run",
		"worker":   "run",
		"schedule": "run",
		"doctor":   "run",
		"upgrade":  "run",
	}

	for _, c := range root.Commands() {
		if g, ok := groupOf[c.Name()]; ok {
			c.GroupID = g
		}
	}

	// Unify the `zever --help` header with the unknown-command usage
	// header (printUsage, main.go): both paths start with usageHeader
	// (suggest.go). Subcommand help keeps the default template.
	root.SetHelpTemplate(usageHeader + "\n\n" + root.HelpTemplate())

	registerCompletions(root)

	return root
}
