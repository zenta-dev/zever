package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/config"
)

// errConfigUnknownSubcommand is returned when `zever config` is given no
// sub-subcommand, or one other than "show".
var errConfigUnknownSubcommand = errors.New("zever config: missing subcommand (want: show)")

// ConfigConfig carries every input runConfigShowWith needs. Screen agents
// build it from huh forms; the flag shell (runConfigShow) builds it from
// argv. Service config always resolves from .env/.env.local discovery plus
// the process environment (see config.FromEnv): there is no --config file
// flag. A nil Out defaults to os.Stdout. JSON emits a machine-readable
// envelope instead of the human adapter list.
type ConfigConfig struct {
	Out  io.Writer
	JSON bool
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
		return fmt.Errorf("%w: %q", ErrUnknownSubcommand, sub)
	}
}

// printConfigUsage prints styled help for `zever config show`.
func printConfigUsage(fs *flag.FlagSet) {
	header := title("zever config show") + dim(" — inspect resolved config")
	usage := bold("Usage:") + "  " + cmd("zever config show")

	body := joinLines(
		header,
		"",
		usage,
		"",
		"Prints the effective env-derived values per instance, redacted.",
		"Named instances come from SERVICE__NAME__* env.",
		"",
		bold("Flags:"),
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(fs.Output(), box(body))
	} else {
		_, _ = fmt.Fprintln(fs.Output(), body)
	}

	printFlagDefaults(fs)
	_, _ = fmt.Fprintln(fs.Output(), "")
	_, _ = fmt.Fprintln(fs.Output(), dim("Examples:"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever config show")+dim("  # print resolved service config + env, redacted"))
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
	fs.Usage = func() { printConfigUsage(fs) }

	if err := rejectUnknownFlags("config show", args, nil); err != nil {
		return err
	}

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("zever config: %w", err)
	}

	return runConfigShowWith(ConfigConfig{JSON: jsonMode})
}

// runConfigShowWith resolves cfg through .env/.env.local discovery plus
// the process environment (matching how the container resolves config) and
// prints one "<service>: adapter=<x>" header per battery, followed by its
// redacted option fields, to cfg.Out. Named instances (SERVICE__NAME__*
// env) print as "<service>/<NAME>" sections with their own effective
// (redacted) values.
//
// No secrets ever reach cfg.Out: the default instance goes through
// config.Config.RedactedServices (the sole display path, replacing every
// sensitive field with config.RedactedValue), and named instances through
// config.Redact over the same JSON-tagged option maps.
func runConfigShowWith(cfg ConfigConfig) error {
	out := outOrStdout(cfg.Out)

	env, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("zever config show: resolve env: %w", err)
	}

	resolved, err := config.Resolve(env)
	if err != nil {
		if hint := envTypoHint(env, err); hint != "" {
			return fmt.Errorf("zever config show: resolve config from env: %w\n%s", err, hint)
		}

		return fmt.Errorf("zever config show: resolve config from env: %w", err)
	}

	services := resolved.RedactedServices()
	instances := redactedInstances(env, resolved, services)

	if cfg.JSON {
		emitSuccess("config show", map[string]any{"services": services, "instances": instances})

		return nil
	}

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

		for _, iname := range sortedInstanceNames(instances, name) {
			isc := instances[name+"/"+iname]
			_, _ = fmt.Fprintf(out, "%s  adapter=%s\n", cyan(name+"/"+iname), isc.Adapter)

			ikeys := make([]string, 0, len(isc.Options))
			for k := range isc.Options {
				ikeys = append(ikeys, k)
			}

			sort.Strings(ikeys)

			for _, k := range ikeys {
				_, _ = fmt.Fprintf(out, "  %s: %v\n", k, isc.Options[k])
			}
		}
	}

	return nil
}

// redactedInstances resolves every named instance (SERVICE__NAME__* env) for
// every service in services, returning "service/NAME" display rows with the
// effective adapter and redacted options. It mirrors the container's own
// overlay (see container/provide.go): DefaultOptions seeded from the service
// default, overlaid by ApplyEnv for the named namespace. A named instance
// whose env fails to apply is skipped: the default row already carries the
// validated config, and doctor (not config show) is the fail-closed surface.
func redactedInstances(env config.Env, resolved *config.Config, services map[string]config.ServiceConfig) map[string]config.ServiceConfig {
	_ = resolved

	out := map[string]config.ServiceConfig{}

	for svc, sc := range services {
		for _, name := range config.InstanceNames(env, svc) {
			seed := config.DefaultOptions(svc, name)
			if seed == nil {
				continue
			}

			// ApplyEnv needs a settable struct pointer of the concrete
			// Options type; DefaultOptions returns the struct value, so a
			// fresh pointer of the same type carries the seed.
			ptr := reflect.New(reflect.TypeOf(seed))
			ptr.Elem().Set(reflect.ValueOf(seed))

			if err := config.ApplyEnv(env, svc, name, ptr.Interface()); err != nil {
				continue
			}

			opts := optionsToMap(ptr.Elem().Interface())

			out[svc+"/"+name] = config.ServiceConfig{
				Adapter: instanceAdapter(env, svc, name, sc.Adapter),
				Options: config.Redact(opts),
			}
		}
	}

	return out
}

// instanceAdapter reports the named instance's adapter: SERVICE__NAME__ADAPTER
// from env (case-insensitive), falling back to the default instance's.
func instanceAdapter(env config.Env, svc, name, def string) string {
	want := strings.ToUpper(svc) + "__" + strings.ToUpper(name) + "_ADAPTER"

	for k, v := range env {
		if strings.ToUpper(k) == want {
			if strings.TrimSpace(v) != "" {
				return v
			}

			break
		}
	}

	return def
}

// sortedInstanceNames lists the instance names under svc in out's
// "service/NAME" keys, sorted for deterministic output.
func sortedInstanceNames(out map[string]config.ServiceConfig, svc string) []string {
	var names []string

	for k := range out {
		if s, n, ok := strings.Cut(k, "/"); ok && s == svc {
			names = append(names, n)
		}
	}

	sort.Strings(names)

	return names
}

// optionsToMap converts typed options to a plain map via the same JSON-tag
// semantics RedactedServices uses. It never fails: unmarshalable values
// yield an empty map rather than leaking raw state.
func optionsToMap(opts any) map[string]any {
	b, err := json.Marshal(opts)
	if err != nil {
		return map[string]any{}
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{}
	}

	return m
}

// envTypoHint turns an UnknownFieldError from Resolve into an exact-key
// hint: it locates the offending env variable (SERVICE_FIELD or
// SERVICE__NAME__FIELD, matched case-insensitively) and names it alongside
// the corrected key, so the operator knows exactly what to rename.
// It returns "" when err is not an unknown-field failure or no set key
// matches, leaving the wrapped error to speak for itself.
func envTypoHint(env config.Env, err error) string {
	var unknown *config.UnknownFieldError
	if !errors.As(err, &unknown) {
		return ""
	}

	head := strings.ToUpper(unknown.Service) + "_"
	field := strings.ToUpper(unknown.Field)

	for k := range env {
		upper := strings.ToUpper(k)
		if !strings.HasPrefix(upper, head) {
			continue
		}

		rest := upper[len(head):]
		if rest == field || strings.HasSuffix(rest, "__"+field) {
			if unknown.Suggestion != "" {
				want := strings.ToUpper(unknown.Service) + "_" + strings.ToUpper(unknown.Suggestion)
				if strings.Contains(rest, "__") {
					inst, _, _ := strings.Cut(rest, "__")
					want = strings.ToUpper(unknown.Service) + "__" + inst + "__" + strings.ToUpper(unknown.Suggestion)
				}

				return formatHint(fmt.Sprintf("check env key %q (did you mean %q?)", k, want))
			}

			return formatHint(fmt.Sprintf("check env key %q", k))
		}
	}

	return ""
}
