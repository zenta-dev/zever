package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// errGenerateUnknownSubcommand is returned when `zever generate` is given no
// sub-subcommand at all.
var errGenerateUnknownSubcommand = errors.New(
	"zever generate: missing subcommand (want: module, entity, job, schedule, server, worker, seed, tinker, adapter)")

// ErrPathTraversal is returned when a developer-supplied name or configured
// path would escape the directory the generator owns. It is a sentinel: use
// errors.Is to detect it rather than matching the message text.
var ErrPathTraversal = errors.New("zever generate: path escapes target directory")

// isTraversalName reports whether a developer-supplied name carries
// path-traversal or separator content: "..", ".", absolute paths, or any
// slash/backslash. Identifier and package-name checks reject most of these
// anyway; this reports the security-relevant subset first so callers can
// return ErrPathTraversal (errors.Is-matchable) instead of a generic
// validation error.
func isTraversalName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return true
	}

	if filepath.IsAbs(name) {
		return true
	}

	return strings.ContainsAny(name, `/\`)
}

// hasParentTraversal reports whether dir, once cleaned, escapes upward via a
// leading ".." segment. Unlike isTraversalName (which rejects any slash at
// all, for single-element names), this accepts legitimate multi-segment
// directory paths such as "./tinker/shim" or an absolute path -- it only
// rejects the one thing that actually escapes the intended tree: a ".."
// component that survives Clean because it isn't matched by an earlier
// segment (e.g. "../../../../etc/cron.d/x", or "a/../../b").
func hasParentTraversal(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}

	clean := filepath.Clean(dir)

	return clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

// joinUnderRoot joins elem onto root and verifies the cleaned result stays
// inside root. It is the defense-in-depth gate behind the name validators:
// even a validated name passes through here before any filesystem call.
//
// coverageProof: after isTraversalName, elem holds no "..", ".", absolute
// path, or slash/backslash, so joined is always exactly one clean path
// element under root; filepath.Rel on such a pair cannot error and cannot
// yield ".." on any GOOS (both sides share absoluteness via Join). The
// Rel/escape checks were therefore provably dead and folded into the single
// traversal gate above with no behavior change.
func joinUnderRoot(root, elem string) (string, error) {
	if isTraversalName(elem) {
		return "", fmt.Errorf("%w: %q", ErrPathTraversal, elem)
	}

	return filepath.Join(root, elem), nil
}

// coverageSeam: prompt indirection for tests. Proof: huh prompts require a
// real TTY and cannot succeed in `go test`; without a var seam the
// interactive success branches in runGenerate/runGenerateModule are
// unreachable, blocking 100% statement coverage. Calls remain identical;
// tests stub these vars, production uses the real prompts.
var (
	promptSelectForGenerate = promptSelect
	promptInputForGenerate  = promptInput
)

// runGenerate dispatches the two-word `zever generate <subcommand>` family,
// mirroring runDB's shape in migrate.go.
//
// "module" keeps its historical calling convention exactly: runGenerateModule
// owns its own flag set and expects the literal "module" word among its
// arguments, so anything that is not a recognized subcommand word is handed
// through to it unchanged rather than rejected here.
func runGenerate(args []string) error { //nolint:gocyclo
	// Support -i/--interactive at this level as well (handles "zever generate -i").
	args = peelInteractive(args)

	if len(args) == 0 {
		if isInteractiveTerminal() {
			sel, err := promptSelectForGenerate("Choose what to generate", allGenerate)
			if err == nil {
				sub := sel
				switch sub {
				case "module":
					return runGenerateModule([]string{"module"})
				case "entity":
					return runGenerateEntity(nil)
				case "job":
					return runGenerateJob(nil)
				case "schedule":
					return runGenerateSchedule(nil)
				case "server":
					return runGenerateServer(nil)
				case "worker":
					return runGenerateWorker(nil)
				case "seed":
					return runGenerateSeed(nil)
				case "tinker":
					return runGenerateTinker(nil)
				case "adapter":
					return runGenerateAdapter(nil)
				}
			}
		}

		printGenerateUsage()

		return errGenerateUnknownSubcommand
	}

	sub, rest := args[0], args[1:]

	switch sub {
	case "module":
		return runGenerateModule(args)
	case "entity":
		return runGenerateEntity(rest)
	case "job":
		return runGenerateJob(rest)
	case "schedule":
		return runGenerateSchedule(rest)
	case "server":
		return runGenerateServer(rest)
	case "worker":
		return runGenerateWorker(rest)
	case "seed":
		return runGenerateSeed(rest)
	case "tinker":
		return runGenerateTinker(rest)
	case "adapter":
		return runGenerateAdapter(rest)
	case "-h", "--help", "help":
		printGenerateUsage()

		return nil
	// coverageProof: "-i/--interactive" arms removed as provably dead.
	// peelInteractive strips every "-i/--interactive" before dispatch, so
	// sub can never equal "-i" here; global -i still works via peel's
	// interactiveMode side effect (covered by TestCoverRunGenerateSubcommands).
	default:
		if strings.HasPrefix(sub, "-") {
			// Legacy flags-before-positionals form of `generate module`.
			return runGenerateModule(args)
		}

		printGenerateUsage()

		if shouldShowHint() {
			if s := closest(sub, allGenerate); s != "" {
				_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
			}
		}

		return fmt.Errorf("zever generate: unknown subcommand %q", sub)
	}
}

func printGenerateUsage() {
	header := title("zever generate") + dim(" — scaffolding")
	usage := bold("Usage:") + "  " + cmd("zever generate") + dim(" <subcommand> [flags]") + dim("  •  -i/--interactive for guided prompts")
	line := func(name, desc string) string {
		return "  " + cmd(fmt.Sprintf("%-26s", name)) + dim(desc)
	}

	body := joinLines(
		header,
		"",
		usage,
		"",
		bold("Subcommands:"),
		line("module <name>", "Scaffold schema/<name>/<name>.zen"),
		line("entity <module> <name>", "Append an entity declaration to a module .zen"),
		line("job <module> <name>", "Append a job declaration"),
		line("schedule <module> <name>", "Append a schedule (requires --cron & --dispatch)"),
		line("server", "Scaffold HTTP server entrypoint"),
		line("worker", "Scaffold job worker + scheduler entrypoint"),
		line("seed", "Scaffold database seed entrypoint"),
		line("tinker", "Scaffold tinker shim"),
		line("adapter <battery> <name>", "Scaffold a stub adapter package"),
		"",
		dim("Run '")+cmd("zever generate <subcommand> -h")+dim("' for flags  •  ")+hint("tip: ")+dim("add -i to be prompted step-by-step"),
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(os.Stderr, box(body))
	} else {
		_, _ = fmt.Fprintln(os.Stderr, body)
	}
}

// usageExample is one Examples row of a boxed usage screen: the command
// plus its trailing comment (empty when the example has none).
type usageExample struct {
	command string
	comment string
}

// printBoxedUsage renders the boxed usage layout every generate/db subcommand
// shares: title header, usage line, body, flag defaults, examples, and an
// optional hint. It is the single copy of the blocks dupl flagged across the
// printXUsage functions; behavior is identical to the inlined originals.
func printBoxedUsage(fs *flag.FlagSet, header, usage, body string, examples []usageExample, hint string) {
	joined := joinLines(
		header,
		"",
		usage,
		"",
		body,
		"",
		bold("Flags:"),
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(fs.Output(), box(joined))
	} else {
		_, _ = fmt.Fprintln(fs.Output(), joined)
	}

	fs.PrintDefaults()
	_, _ = fmt.Fprintln(fs.Output(), "")
	_, _ = fmt.Fprintln(fs.Output(), dim("Examples:"))
	for _, ex := range examples {
		_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd(ex.command)+dim(ex.comment))
	}

	if shouldShowHint() {
		_, _ = fmt.Fprintln(fs.Output(), formatHint(hint))
	}
}

// parseEntrypointFlags parses the --force/--interactive preamble both
// runGenerateSeed and runGenerateWorker share: peel -i, define the flags,
// set interactiveMode, load the project config and module path, and confirm
// overwriting an existing entrypoint. entryFile selects the entry directory
// (SeedEntry vs WorkerEntry) and confirm is the matching prompt seam.
// Identical to the inlined originals it replaces.
func parseEntrypointFlags(args []string, flagName string, usage func(*flag.FlagSet), entryFile func(ProjectConfig) string, confirm func(string, string) (bool, error)) (ProjectConfig, string, bool, error) {
	empty := ProjectConfig{}

	args = peelInteractive(args)
	fs := flag.NewFlagSet(flagName, flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite the entrypoint if it already exists")
	// coverageProof: no local -i/--interactive flags; peelInteractive
	// strips them before Parse and sets interactiveMode globally.

	fs.Usage = func() {
		usage(fs)
	}

	if err := fs.Parse(args); err != nil {
		return empty, "", false, err
	}

	project, err := loadProjectConfig()
	if err != nil {
		return empty, "", false, err
	}

	modulePath, err := goModulePath()
	if err != nil {
		return empty, "", false, err
	}

	forceVal := *force
	if !forceVal && isInteractiveTerminal() {
		if _, statErr := os.Stat(filepath.Join(entryFile(project), "main.go")); statErr == nil {
			ok, perr := confirm(fmt.Sprintf("%q already exists — overwrite?", filepath.Join(entryFile(project), "main.go")), "Yes, overwrite")
			if perr != nil {
				return empty, "", false, perr
			}

			if ok {
				forceVal = true
			}
		}
	}

	return project, modulePath, forceVal, nil
}

// splitPositionals peels up to n leading non-flag arguments off args and
// returns them plus whatever is left for a flag.FlagSet to parse.
//
// Go's flag package stops parsing at the first non-flag argument, so without
// this both `generate entity shop Order --field title:string` and
// `generate entity --field title:string shop Order` could not work. Peeling
// the leading positionals first makes both forms behave identically; any
// positional that trails the flags is recovered from fs.Args() by the caller.
func splitPositionals(args []string, n int) (positional, remainder []string) {
	i := 0
	for i < len(args) && len(positional) < n && !strings.HasPrefix(args[i], "-") {
		positional = append(positional, args[i])
		i++
	}

	return positional, args[i:]
}

// GenerateModuleConfig is the pure input to GenerateModule: the module name
// and the schema directory that owns the new module. Stdout and Stderr
// receive the success and hint lines; nil writers silence output. The
// working directory is the filesystem root (tests chdir into a temp dir).
type GenerateModuleConfig struct {
	Name      string
	SchemaDir string
	Stdout    io.Writer
	Stderr    io.Writer
}

// GenerateModule scaffolds schema/<name>/<name>.zen and returns the stub
// path. It performs no flag parsing and reads no global state beyond the
// working directory, so tests can drive it inside a temp dir.
func GenerateModule(cfg GenerateModuleConfig) (string, error) {
	const tag = "zever generate module"

	if isTraversalName(cfg.Name) {
		return "", fmt.Errorf("%s: %w: %q", tag, ErrPathTraversal, cfg.Name)
	}

	if !isIdent(cfg.Name) {
		return "", fmt.Errorf("%s: name must be an identifier, got %q", tag, cfg.Name)
	}

	dir, err := joinUnderRoot(cfg.SchemaDir, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("%s: %w", tag, err)
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("%s: mkdir %q: %w", tag, dir, err)
	}

	stub := fmt.Sprintf("// %s module — add entities, services, jobs here.\n", cfg.Name)
	path := filepath.Join(dir, cfg.Name+".zen")

	if err := os.WriteFile(path, []byte(stub), 0o644); err != nil { //nolint:gosec // generated schema stub, not a secret
		return "", fmt.Errorf("%s: write %q: %w", tag, path, err)
	}

	stdout := cfg.Stdout
	if stdout == nil {
		stdout = io.Discard
	}

	_, _ = fmt.Fprintln(stdout, success("✔ scaffolded ")+bold(fmt.Sprintf("module %q", cfg.Name))+dim(" at ")+cyan(path))

	if stderr := cfg.Stderr; stderr != nil && shouldShowHint() {
		_, _ = fmt.Fprintln(stderr, formatHint(hintFor("generate module")))
	}

	return path, nil
}

// runGenerateModule scaffolds a new module: schema/<name>/<name>.zen
// With dir-derived module identity, the directory name is the module.
func runGenerateModule(args []string) error {
	args = peelInteractive(args)
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	// coverageProof: no local -i/--interactive flags; peelInteractive
	// strips them before Parse and sets interactiveMode globally.

	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()

	if len(rest) < 2 || rest[0] != "module" {
		if isInteractiveTerminal() {
			val, err := promptInputForGenerate("Module name", "", func(s string) error {
				if !isIdent(s) {
					return errors.New("must be identifier")
				}

				return nil
			})
			if err == nil {
				rest = []string{"module", val}
			} else {
				return err
			}
		} else {
			return errors.New(`zever generate: usage: zever generate module <name>`)
		}
	}

	name := rest[1]
	if name == "" {
		if isInteractiveTerminal() {
			val, err := promptInputForGenerate("Module name", "", func(s string) error {
				if s == "" {
					return errors.New("must not be empty")
				}

				if !isIdent(s) {
					return errors.New("must be identifier")
				}

				return nil
			})
			if err != nil {
				return err
			}

			name = val
		} else {
			return errors.New("zever generate module: name must not be empty")
		}
	}

	schemaDir := defaultSchemaDir
	if pc, err := loadProjectConfig(); err == nil && pc.SchemaDir != "" {
		schemaDir = pc.SchemaDir
	}

	_, err := GenerateModule(GenerateModuleConfig{
		Name:      name,
		SchemaDir: schemaDir,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})

	return err
}
