package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// newDBCmd returns the `db` parent with migrate/rollback/seed subcommands.
// Each subcommand wraps runDB with fixed sub-args; the stdlib flag parsing
// inside runDBMigrate/runDBRollback/runDBSeed stays authoritative, so every
// subcommand sets DisableFlagParsing and passes raw args through. The bare
// parent delegates to runDB so `zever db` keeps its missing-subcommand
// usage + error contract.
func newDBCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "db",
		Short: "Database commands (migrate, rollback, seed)",
		Long: `Database commands: create tables from .zen schemas
(migrate), undo the most recent migration statements (rollback),
and run the seed entrypoint (seed).`,
		Example: `  zever db migrate --adapter=sqlite --dsn=data/app.db schema/*.zen`,
		Args:    cobra.ArbitraryArgs,
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

			return runDB(args)
		},
	}

	parent.AddCommand(
		&cobra.Command{
			Use:   "migrate",
			Short: "Create tables from .zen schemas",
			Long: `Create tables from .zen schemas in the target database
(supports --dry-run and -i for guided DSN/adapter prompts).`,
			Example:            `  zever db migrate schema/app.zen --adapter=sqlite --dsn=data/app.db`,
			Args:               cobra.ArbitraryArgs,
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if versionRequested(cmd) {
					printCLIVersion()

					return nil
				}

				return runDB(append([]string{"migrate"}, args...))
			},
		},
		&cobra.Command{
			Use:   "rollback",
			Short: "Undo the most recently applied migration statements",
			Long: `Undo the most recently applied migration statements
recorded in the migration journal.`,
			Example:            `  zever db rollback --adapter=sqlite --dsn=data/app.db schema/*.zen`,
			Args:               cobra.ArbitraryArgs,
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if versionRequested(cmd) {
					printCLIVersion()

					return nil
				}

				return runDB(append([]string{"rollback"}, args...))
			},
		},
		&cobra.Command{
			Use:   "seed",
			Short: "Run seed entrypoint (default db/seed)",
			Long: `Seed the database by running the seed entrypoint
package (project.seed_entry, default db/seed). Every argument is
passed through to that package.`,
			Example:            `  zever db seed`,
			Args:               cobra.ArbitraryArgs,
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if versionRequested(cmd) {
					printCLIVersion()

					return nil
				}

				return runDB(append([]string{"seed"}, args...))
			},
		},
	)

	return parent
}
