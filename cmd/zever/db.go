package main

import (
	"errors"
	"fmt"
	"os"
)

// errDBUnknownSubcommand is returned when `zever db` is given no
// sub-subcommand at all.
var errDBUnknownSubcommand = errors.New("zever db: missing subcommand (want: migrate, rollback, seed)")

// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// without a var seam the interactive success branches are unreachable in `go
// test`, blocking 100% statement coverage. Behavior unchanged.
var promptSelectForDB = promptSelect

// runDB dispatches the two-word `zever db <subcommand>` family.
func runDB(args []string) error {
	args = peelInteractive(args)
	if len(args) == 0 {
		if isInteractiveTerminal() {
			sel, err := promptSelectForDB("Choose db action", allDB)
			if err == nil {
				switch sel {
				case "migrate":
					return runDBMigrate(nil)
				case "rollback":
					return runDBRollback(nil)
				case "seed":
					return runDBSeed(nil)
				}
			}
		}

		printDBUsage()

		return errDBUnknownSubcommand
	}

	sub, rest := args[0], args[1:]

	switch sub {
	case "migrate":
		return runDBMigrate(rest)
	case "rollback":
		return runDBRollback(rest)
	case "seed":
		return runDBSeed(rest)
	case "-h", "--help", "help":
		printDBUsage()
		return nil
	// coverageProof: "-i/--interactive" arm removed as provably dead.
	// peelInteractive strips every "-i/--interactive" before dispatch, so
	// sub can never equal "-i" here; global -i still works via peel.
	default:
		printDBUsage()

		if shouldShowHint() {
			if s := closest(sub, allDB); s != "" {
				_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
			}
		}

		return fmt.Errorf("zever db: unknown subcommand %q", sub)
	}
}

func printDBUsage() {
	header := title("zever db") + dim(" — database")
	usage := bold("Usage:") + "  " + cmd("zever db") + dim(" <subcommand> [flags]")
	line := func(name, desc string) string {
		return "  " + cmd(fmt.Sprintf("%-12s", name)) + dim(desc)
	}

	body := joinLines(
		header,
		"",
		usage,
		"",
		bold("Subcommands:"),
		line("migrate", "Create tables from .zen schemas (supports --dry-run)"),
		line("rollback", "Undo the most recently applied migration statements"),
		line("seed", "Run seed entrypoint (default db/seed)"),
		"",
		dim("Run '")+cmd("zever db <subcommand> -h")+dim("' for flags  •  ")+hint("tip: ")+dim("add -i for guided DSN/adapter prompts"),
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(os.Stderr, box(body))
	} else {
		_, _ = fmt.Fprintln(os.Stderr, body)
	}
}
