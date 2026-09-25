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
	"new", "generate", "extract",
	"compile", "check", "breaking", "fmt", "doctor", "config",
	"routes", "explain", "check-boundaries", "check:boundaries", "graph",
	"serve", "dev", "queue:work", "schedule:run", "tinker",
	"db", "completion", "docs",
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

// globalFlagToken reports whether tok is one of the boolean global flags that
// may precede the subcommand. Used only to locate the first positional for
// Cobra-vs-legacy routing; Cobra itself does the real parsing.
func globalFlagToken(tok string) bool {
	switch tok {
	case "-i", "--interactive", "-q", "--quiet", "--no-color", "-V", "--version":
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
	_, _ = fmt.Fprintln(os.Stdout, "zever v"+cliVersion)
}

// newRootCmd builds the Cobra root: usage metadata, silenced usage/errors
// (runtime errors print via main, never with a usage dump), the CLI version,
// persistent global flags, and the ported command set.
func newRootCmd() *cobra.Command {
	var interactive, quiet, noColor, showV bool

	root := &cobra.Command{
		Use:   "zever",
		Short: "Zever application toolkit",
		Long: `Zever application toolkit: scaffold, inspect, run, and migrate
zever applications built from .zen schemas.`,
		Version:            cliVersion,
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
				printUsage()

				return errMissingSubcommand
			}

			// Positional under bare root (only reachable when useCobra
			// misroutes, e.g. direct Execute in tests): keep legacy shape.
			printUsage()

			if shouldShowHint() {
				if s := closest(args[0], allTopLevel); s != "" {
					_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
				}
			}

			return fmt.Errorf("zever: unknown subcommand %q", args[0])
		},
	}

	root.PersistentFlags().BoolVarP(&interactive, "interactive", "i", false, "guided prompts where supported (also ZEVER_INTERACTIVE=1)")
	root.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress non-essential output")
	root.PersistentFlags().Bool("no-color", false, "disable ANSI styling")
	root.PersistentFlags().BoolVar(&showV, "V", false, "print the CLI version (alias for --version)")

	root.PersistentPreRun = func(_ *cobra.Command, _ []string) {
		if interactive || envInteractive() {
			interactiveMode = true
		}

		if noColor {
			colorEnabled = false
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

	root.AddCommand(newDBCmd())

	for _, c := range newInspectCmds() {
		root.AddCommand(c)
	}

	root.AddCommand(newCompletionCmd(root))
	root.AddCommand(newDocsCmd(root))

	return root
}
