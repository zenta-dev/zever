package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// newScaffoldCmds returns the scaffolding subcommands: new, generate,
// extract. Each wraps its legacy runX entry point without touching the
// stdlib flag parsing inside: DisableFlagParsing passes raw args (including
// -h) straight through so existing usage printers keep working.
func newScaffoldCmds() []*cobra.Command {
	return []*cobra.Command{newNewCmd(), newGenerateCmd(), newExtractCmd()}
}

func newNewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a brand new zever application",
		Long: `Scaffold a brand new, buildable zever application from scratch:
a fresh go.mod, a starter schema/app.zen, cmd/server + cmd/worker +
db/seed entrypoints, and internal/app wiring.`,
		Example: `  zever new myapp`,
		Args:    cobra.ArbitraryArgs,
		// DisableFlagParsing keeps the legacy stdlib flag set (including
		// --module, --dir, --framework-version, --force, -h) authoritative.
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

			return runNew(args)
		},
	}
}

func newGenerateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate <subcommand>",
		Short: "Scaffold modules, entities, jobs, schedules, entrypoints",
		Long: `Scaffold modules, entities, jobs, schedules, server/worker/seed
entrypoints, tinker shims, and stub adapter packages.`,
		Example: `  zever generate entity shop Order --field title:string`,
		Args:    cobra.ArbitraryArgs,
		// DisableFlagParsing keeps runGenerate's two-word dispatch and its
		// per-subcommand stdlib flag sets authoritative.
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

			return runGenerate(args)
		},
	}
}

func newExtractCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "extract <module>",
		Short: "Extract one module into a standalone service",
		Long: `Extract one schema module into a standalone, separately
deployable Go module: its own go.mod, the declaring .zen files, the
generated ORM package for its entities, and scoped entrypoints.`,
		Example: `  zever extract shop --out ./shop-service`,
		Args:    cobra.ArbitraryArgs,
		// DisableFlagParsing keeps runExtract's stdlib flag set (--out,
		// --module, --force, -h) authoritative.
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

			return runExtract(args)
		},
	}
}
