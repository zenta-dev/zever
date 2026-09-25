package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// newInspectCmds builds the schema-inspection pass-through commands.
//
// Each command delegates to its existing runXxx(args) handler without
// rewriting flag parsing (DisableFlagParsing keeps -i and positional flags
// flowing to the legacy handlers untouched). The RunE wrapper pre-scans args
// for -h/--help/help and returns cmd.Help() first, fixing the known bug
// where handlers treated "-h" as a .zen file (e.g. `routes -h` tried to read
// a file named "-h").
func newInspectCmds() []*cobra.Command {
	compileCmd := &cobra.Command{
		Use:   "compile [--backend atlas,gogen,openapi,proto,protogogen,zenorm] [--out DIR] <files...>",
		Short: "Compile .zen schemas through backends",
		Long:  "Compile .zen schema files through one or more backends, writing each backend's output under <out>/<backend>/.",
		Example: `  zever compile schema/app.zen --backend=proto
  zever compile --backend=zenorm,proto,atlas,openapi schema/*.zen --out ./generated
  zever compile -i`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := printHelpIfRequested(cmd, args); err != nil {
				if errors.Is(err, errHelpShown) {
					return nil
				}

				return err
			}

			return runCompile(args)
		},
	}

	checkCmd := &cobra.Command{
		Use:   "check <files...>",
		Short: "Validate .zen schemas without writing output",
		Long:  "Validate .zen schema files with zero backends (schema resolution only, no codegen, no filesystem writes).",
		Example: `  zever check schema/app.zen
  zever check schema/*.zen`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := printHelpIfRequested(cmd, args); err != nil {
				if errors.Is(err, errHelpShown) {
					return nil
				}

				return err
			}

			return runCheck(args)
		},
	}

	breakingCmd := &cobra.Command{
		Use:   "breaking <old-files...> -- <new-files...>",
		Short: "Report API-breaking changes between two schema versions",
		Long:  "Compile two versions of a schema separately and report every API-breaking change between them.",
		Example: `  zever breaking schema-old/*.zen -- schema/*.zen
  git show main:schema/app.zen > /tmp/old.zen && zever breaking /tmp/old.zen -- schema/app.zen`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := printHelpIfRequested(cmd, args); err != nil {
				if errors.Is(err, errHelpShown) {
					return nil
				}

				return err
			}

			return runBreaking(args)
		},
	}

	fmtCmd := &cobra.Command{
		Use:   "fmt [-l | -w] <files...>",
		Short: "Format .zen schemas",
		Long:  "Format .zen schema files, mirroring gofmt: list files that would change by default, rewrite in place with -w/--write.",
		Example: `  zever fmt schema/*.zen
  zever fmt --write schema/*.zen`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := printHelpIfRequested(cmd, args); err != nil {
				if errors.Is(err, errHelpShown) {
					return nil
				}

				return err
			}

			return runFmt(args)
		},
	}

	doctorCmd := &cobra.Command{
		Use:   "doctor [--config PATH] [--strict]",
		Short: "Verify every battery resolves",
		Long:  "Verify batteries by resolving every service from config and reporting OK/FAIL per battery.",
		Example: `  zever doctor
  zever doctor --config zever.yaml
  zever doctor -i`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := printHelpIfRequested(cmd, args); err != nil {
				if errors.Is(err, errHelpShown) {
					return nil
				}

				return err
			}

			return runDoctor(args)
		},
	}

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect resolved config",
		Long:  "Inspect the resolved zever config (redacted).",
		Example: `  zever config show
  zever config show --config zever.yaml`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, a := range args {
				if a == "-h" || a == "--help" || a == "help" {
					return cmd.Help()
				}
			}

			return runConfig(args)
		},
	}

	configShowCmd := &cobra.Command{
		Use:   "show [--config PATH]",
		Short: "Print the resolved config, redacted",
		Long:  "Print the resolved config, redacted, one adapter header per battery with redacted option fields.",
		Example: `  zever config show
  zever config show --config zever.yaml`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := printHelpIfRequested(cmd, args); err != nil {
				if errors.Is(err, errHelpShown) {
					return nil
				}

				return err
			}

			return runConfigShow(args)
		},
	}
	configCmd.AddCommand(configShowCmd)

	routesCmd := &cobra.Command{
		Use:   "routes <files...>",
		Short: "List HTTP routes declared by RPCs",
		Long:  "List HTTP routes declared by RPCs, one line per RPC with an HTTP binding.",
		Example: `  zever routes schema/app.zen
  zever routes schema/*.zen`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := printHelpIfRequested(cmd, args); err != nil {
				if errors.Is(err, errHelpShown) {
					return nil
				}

				return err
			}

			return runRoutes(args)
		},
	}

	explainCmd := &cobra.Command{
		Use:   "explain <Service.Operation|Module.Service.Operation> <files...>",
		Short: "Print an operation's declaration location and summary",
		Long:  "Print an operation's declaration location and a compact summary (transports, errors, auth, permission).",
		Example: `  zever explain UserService.GetUser schema/app.zen
  zever explain billing.OrderService.GetOrder schema/*.zen`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := printHelpIfRequested(cmd, args); err != nil {
				if errors.Is(err, errHelpShown) {
					return nil
				}

				return err
			}

			return runExplain(args)
		},
	}

	boundariesCmd := &cobra.Command{
		Use:                "check-boundaries <files...>",
		Aliases:            []string{"check:boundaries"},
		Short:              "Report cross-module reference violations",
		Long:               "Report every module-boundary violation the resolver detects during a normal Resolve().",
		Example:            `  zever check-boundaries schema/*.zen`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := printHelpIfRequested(cmd, args); err != nil {
				if errors.Is(err, errHelpShown) {
					return nil
				}

				return err
			}

			return runCheckBoundaries(args)
		},
	}

	graphCmd := &cobra.Command{
		Use:                "graph <files...>",
		Short:              "Print Mermaid entity-relation and module diagrams",
		Long:               "Print the resolved schema as Mermaid diagrams: one entity-relation graph per module, then one module-to-module graph.",
		Example:            `  zever graph schema/*.zen`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := printHelpIfRequested(cmd, args); err != nil {
				if errors.Is(err, errHelpShown) {
					return nil
				}

				return err
			}

			return runGraph(args)
		},
	}

	return []*cobra.Command{
		compileCmd,
		checkCmd,
		breakingCmd,
		fmtCmd,
		doctorCmd,
		configCmd,
		routesCmd,
		explainCmd,
		boundariesCmd,
		graphCmd,
	}
}
