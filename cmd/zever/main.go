// Command zever is the zever CLI dispatch shell: a thin, TUI-first entry
// point that routes subcommands to their handlers and yields to the
// interactive dashboard when invoked bare on a TTY.
//
// This file owns the run(args) → error seam (thin main → run, os.Exit on
// error, stderr only), the errInteractive sentinel the TUI agent wires the
// dashboard to, the subcommand dispatch table, and the top-level usage text.
// Handler implementations (runNew, runCompile, …) land via parallel agents;
// each table entry names its handler exactly so the wiring is a pure rename.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// errInteractive is returned by run when zever is invoked with no subcommand
// on an interactive terminal. main maps it to launching the dashboard (the
// TUI agent owns that branch); any other caller can use errors.Is to detect
// the "drop to interactive mode" case.
var errInteractive = errors.New("zever: interactive session requested")

// errMissingSubcommand is returned by run when zever is invoked with no
// subcommand and stdin is not a terminal, after usage is printed.
var errMissingSubcommand = errors.New("zever: missing subcommand")

// isStdinTerminal reports whether stdin is an interactive terminal.
//
// It is a seam var (not a direct term.IsTerminal call) so tests can override
// it and so this file stays stdlib-only: golang.org/x/term is not a direct
// dependency. The default is a conservative os.ModeCharDevice check — it can
// false-positive on /dev/null (a char device that is not a TTY), which is the
// safe direction here (worst case: errInteractive on a headless box, which
// main surfaces explicitly). Swap the body for term.IsTerminal once x/term is
// a direct dependency.
var isStdinTerminal = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}

// interactiveMode holds whether -i/--interactive was requested (global or
// ZEVER_INTERACTIVE env). Per-command FlagSets alias it; prompt wiring lives
// with the handler agents.
var interactiveMode bool

// envInteractive returns true if ZEVER_INTERACTIVE=1/true/yes (case-insensitive).
func envInteractive() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ZEVER_INTERACTIVE")))
	return v == "1" || v == "true" || v == "yes"
}

// peelInteractive scans args for -i/--interactive, sets interactiveMode, and
// returns the filtered args. Supports both "zever -i new" and "zever new -i".
func peelInteractive(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a == "-i" || a == "--interactive" {
			interactiveMode = true
			continue
		}

		filtered = append(filtered, a)
	}
	// Also respect env.
	if envInteractive() {
		interactiveMode = true
	}

	return filtered
}

// subcommandHandlers is the dispatch table: top-level subcommand name →
// handler. Handler bodies (runNew, runCompile, …) are implemented by parallel
// agents; this table names each one exactly. Keep in sync with allTopLevel
// (suggest.go) minus the "help" alias, which run handles inline.
//
// Separator standard: single words bare (`check`, `graph`); multi-word
// single commands hyphenated (`check-boundaries`, alias `check:boundaries`);
// namespaced runtime commands colon-separated (`queue:work`,
// `schedule:run`); sub-resources space-separated (`config show`, `db migrate`).
var subcommandHandlers = map[string]func([]string) error{
	"new":              runNew,
	"compile":          runCompile,
	"doctor":           runDoctor,
	"config":           runConfig,
	"routes":           runRoutes,
	"check":            runCheck,
	"breaking":         runBreaking,
	"fmt":              runFmt,
	"explain":          runExplain,
	"check-boundaries": runCheckBoundaries,
	"check:boundaries": runCheckBoundaries,
	"graph":            runGraph,
	"generate":         runGenerate,
	"extract":          runExtract,
	"serve":            runServe,
	"dev":              runDev,
	"queue:work":       runQueueWork,
	"schedule:run":     runScheduleRun,
	"tinker":           runTinker,
	"db":               runDB,
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errInteractive) {
			// TUI-first: bare `zever` on a TTY drops into the
			// dashboard (see dashboard.go: single shell program,
			// model swap on select, esc-back pops). Exit status
			// reflects the session: 0 on clean quit.
			if derr := runDashboard(); derr != nil {
				if colorEnabled {
					_, _ = fmt.Fprintln(os.Stderr, red(derr.Error()))
				} else {
					_, _ = fmt.Fprintln(os.Stderr, derr)
				}

				os.Exit(1)
			}

			os.Exit(0)
		}

		if colorEnabled {
			_, _ = fmt.Fprintln(os.Stderr, red(err.Error()))
		} else {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}

		os.Exit(1)
	}
}

func run(args []string) error {
	// Support global -i/--interactive before subcommand.
	args = peelInteractive(args)

	if len(args) == 0 {
		if isStdinTerminal() {
			return errInteractive
		}

		printUsage()

		return errMissingSubcommand
	}

	sub, rest := args[0], args[1:]

	if h, ok := subcommandHandlers[sub]; ok {
		return h(rest)
	}

	switch sub {
	case "help":
		if len(rest) > 0 {
			// Dispatch `zever help <subcommand>` to that subcommand's help.
			switch rest[0] {
			case "new":
				// new has no dedicated print func; show top usage with hint.
				printUsage()

				_, _ = fmt.Fprintln(os.Stderr, formatHint("try 'zever new -h' for full flags")) //nolint:wsl_v5

				return nil
			case "compile":
				printCompileUsage(flag.NewFlagSet("compile", flag.ContinueOnError))
				return nil
			case "check":
				printCheckUsage(flag.NewFlagSet("check", flag.ContinueOnError))
				return nil
			case "breaking":
				printBreakingUsage(flag.NewFlagSet("breaking", flag.ContinueOnError))
				return nil
			case "fmt":
				printFmtUsage(flag.NewFlagSet("fmt", flag.ContinueOnError))
				return nil
			case "doctor":
				printDoctorUsage(flag.NewFlagSet("doctor", flag.ContinueOnError))
				return nil
			case "config":
				printConfigUsage(flag.NewFlagSet("config", flag.ContinueOnError))
				return nil
			case "generate":
				printGenerateUsage()
				return nil
			case "db":
				printDBUsage()
				return nil
			case "extract":
				printExtractUsage(flag.NewFlagSet("extract", flag.ContinueOnError))
				return nil
			case "serve":
				return runServe([]string{"-h"})
			case "dev":
				printDevUsage(os.Stderr)
				return nil
			case "tinker":
				printTinkerUsage(flag.NewFlagSet("tinker", flag.ContinueOnError))
				return nil
			}
		}

		printUsage()

		return nil
	case "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()

		if shouldShowHint() {
			if s := closest(sub, allTopLevel); s != "" {
				_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
			} else {
				_, _ = fmt.Fprintln(os.Stderr, formatHint("run 'zever --help' for usage"))
			}
		}

		return fmt.Errorf("zever: unknown subcommand %q", sub)
	}
}

func printUsage() {
	header := title("zever") + dim(" — zever toolkit")
	usage := bold("Usage:") + "  " + cmd("zever") + dim(" <command> [flags]") + dim("  •  ") + dim("add -i/--interactive for guided prompts")

	sectionScaffold := bold("Scaffolding")
	sectionInspect := bold("Inspection")
	sectionRuntime := bold("Runtime")
	sectionDB := bold("Database")

	// Columns: command (cyan, padded) + dim description.
	line := func(name, desc string) string {
		return "  " + cmd(fmt.Sprintf("%-18s", name)) + dim(desc)
	}

	body := joinLines(
		header,
		"",
		usage,
		"",
		sectionScaffold,
		line("new", "Scaffold a brand new zever application"),
		line("generate", "Scaffold modules, entities, jobs, schedules, entrypoints"),
		line("extract", "Extract one module into a standalone service"),
		"",
		sectionInspect,
		line("compile", "Compile .zen schemas through backends (atlas, gogen, openapi, proto, protogogen, zenorm; default: all)"),
		line("check", "Validate .zen schemas only — no output written"),
		line("breaking", "Report API-breaking changes between two schema versions"),
		line("fmt", "Format .zen schemas (gofmt-style: -l list, --write rewrite)"),
		line("doctor", "Resolve every battery from config and report "+successMark()+"/"+failMark()),
		line("config show", "Print the resolved config, redacted"),
		line("routes", "List HTTP routes declared by RPCs"),
		line("explain", "Print an operation's declaration location and summary"),
		line("check-boundaries", "Report cross-module reference violations"),
		line("graph", "Print Mermaid entity-relation and module diagrams"),
		"",
		sectionRuntime,
		line("serve", "Run server entrypoint (default cmd/server)"),
		line("dev", "Watch schemas & source; recompile and restart"),
		line("queue:work", "Run worker entrypoint (default cmd/worker)"),
		line("schedule:run", "Alias for queue:work (worker runs scheduler)"),
		line("tinker", "Live container REPL via tinker shim"),
		"",
		sectionDB,
		line("db", "Database commands (db migrate, db rollback, db seed)"),
		"",
		bold("Examples:"),
		dim("  ")+cmd("zever new myapp")+dim("                  # scaffold new app"),
		dim("  ")+cmd("zever generate entity shop Order --field title:string")+dim("  # add entity"),
		dim("  ")+cmd("zever compile schema/*.zen --backend=proto,zenorm")+dim("  # compile"),
		dim("  ")+cmd("zever db migrate --adapter=sqlite --dsn=data/app.db schema/*.zen")+dim("  # migrate"),
		dim("  ")+cmd("zever dev")+dim("                       # watch & serve"),
		"",
		dim("Run '")+cmd("zever <command> -h")+dim("' for subcommand flags  •  ")+hint("tip: ")+dim(generalTips[2]),
		dim("Add ")+cmd("-i / --interactive")+dim(" for guided prompts  •  ")+dim("set ")+cmd("ZEVER_NO_HINT=1")+dim(" to silence hints"),
	)
	// Optionally wrap in a subtle box when color enabled and wide terminal.
	if colorEnabled {
		_, _ = fmt.Fprintln(os.Stderr, box(body))
	} else {
		_, _ = fmt.Fprintln(os.Stderr, body)
	}
}
