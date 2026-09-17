package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/zenta-dev/zever/config"
)

// errConfigUnknownSubcommand is returned when `zever config` is given no
// sub-subcommand, or one other than "show".
var errConfigUnknownSubcommand = errors.New("zever config: missing subcommand (want: show)")

// ConfigConfig carries every input runConfigShowWith needs. Screen agents
// build it from huh forms; the flag shell (runConfigShow) builds it from
// argv. An empty ConfigPath verifies config.Default(). A nil Out defaults to
// os.Stdout.
type ConfigConfig struct {
	ConfigPath string
	Out        io.Writer
}

// runConfig dispatches the two-word `zever config <subcommand>` family.
// Only "show" exists today; unknown or missing subcommands print usage and
// return errConfigUnknownSubcommand.
func runConfig(args []string) error {
	if len(args) == 0 {
		printConfigUsage(flag.NewFlagSet("config", flag.ContinueOnError))
		return errConfigUnknownSubcommand
	}

	sub, rest := args[0], args[1:]

	switch sub {
	case "show":
		return runConfigShow(rest)
	case "-h", "--help", "help":
		printConfigUsage(flag.NewFlagSet("config", flag.ContinueOnError))
		return nil
	default:
		printConfigUsage(flag.NewFlagSet("config", flag.ContinueOnError))
		return fmt.Errorf("zever config: unknown subcommand %q", sub)
	}
}

// printConfigUsage prints styled help for `zever config show`.
func printConfigUsage(fs *flag.FlagSet) {
	header := title("zever config show") + dim(" — inspect resolved config")
	usage := bold("Usage:") + "  " + cmd("zever config show") + dim(" [--config PATH]")

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
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever config show")+dim("  # print config.Default(), redacted"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever config show --config zever.yaml"))
}

// runConfigShow builds a ConfigConfig from argv and runs runConfigShowWith.
func runConfigShow(args []string) error {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("config show", flag.ContinueOnError)
		fs.Usage = func() { printConfigUsage(fs) }
		fs.Usage()

		return nil
	}

	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	configPath := fs.String("config", "", "optional config file path (default: config.Default(), zero-infra)")
	fs.Usage = func() { printConfigUsage(fs) }

	if err := fs.Parse(args); err != nil {
		return err
	}

	return runConfigShowWith(ConfigConfig{ConfigPath: *configPath})
}

// runConfigShowWith resolves cfg (config.Default when ConfigPath is empty,
// config.Load(ConfigPath) otherwise) and prints one "<service>: adapter=<x>"
// header per battery, followed by its redacted option fields, to cfg.Out.
//
// No secrets ever reach cfg.Out: config.Config.RedactedServices is the sole
// display path, and it replaces every sensitive field with
// config.RedactedValue before this function ever sees it.
func runConfigShowWith(cfg ConfigConfig) error {
	out := outOrStdout(cfg.Out)

	resolved := config.Default()

	if cfg.ConfigPath != "" {
		loaded, err := config.Load(cfg.ConfigPath)
		if err != nil {
			return fmt.Errorf("zever config show: load config %q: %w", cfg.ConfigPath, err)
		}

		resolved = loaded
	}

	services := resolved.RedactedServices()

	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		sc := services[name]
		_, _ = fmt.Fprintf(out, "%s  adapter=%s\n", cyan(name), sc.Adapter)

		keys := make([]string, 0, len(sc.Options))
		for k := range sc.Options {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			_, _ = fmt.Fprintf(out, "  %s: %v\n", k, sc.Options[k])
		}
	}

	return nil
}
