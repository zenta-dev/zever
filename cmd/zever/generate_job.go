package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const jobUsageBody = `Appends a job declaration to internal/<module>/<module>.zen with a default
retry policy (3 attempts, exponential backoff from 30s). The job is declared
with an empty parameter list; add parameters by editing the .zen file.`

// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// var seams keep behavior identical while enabling success/error coverage.
//
//nolint:dupl
var (
	promptSelectForJob = promptSelect
	promptInputForJob  = promptInput
)

func printJobUsage(fs *flag.FlagSet) {
	header := title("zever generate job") + dim(" — scaffold a job")
	//nolint:lll
	usage := bold("Usage:") + "  " + cmd("zever generate job") + dim(" <module> <name> [--queue name]") + dim("  •  -i/--interactive for guided prompts")

	printBoxedUsage(fs, header, usage, jobUsageBody, []usageExample{
		{command: "zever generate job shop SendEmail --queue default"},
		{command: "zever generate job -i", comment: "  # guided: select module, name & queue"},
	}, "add -i to be prompted step-by-step")
}

// GenerateJobConfig is the pure input to GenerateJob: the target module, the
// new job name, and the queue it runs on.
type GenerateJobConfig struct {
	Module string
	Name   string
	Queue  string
	Stdout io.Writer
	Stderr io.Writer
}

// GenerateJob appends the rendered job declaration to the module's .zen file
// and returns the file path. It performs no flag parsing.
func GenerateJob(cfg GenerateJobConfig) (string, error) {
	const tag = "zever generate job"

	if isTraversalName(cfg.Module) || isTraversalName(cfg.Name) {
		bad := cfg.Module
		if isTraversalName(cfg.Name) {
			bad = cfg.Name
		}

		return "", fmt.Errorf("%s: %w: %q", tag, ErrPathTraversal, bad)
	}

	if !isIdent(cfg.Module) || !isIdent(cfg.Name) {
		return "", fmt.Errorf("%s: module and job name must be identifiers, got %q and %q", tag, cfg.Module, cfg.Name)
	}

	// The grammar's `queue:` option takes a bare identifier, not a string.
	if !isIdent(cfg.Queue) {
		return "", fmt.Errorf("%s: --queue %q must be an identifier", tag, cfg.Queue)
	}

	path, _, decl, err := readZenModule(tag, cfg.Module)
	if err != nil {
		return "", err
	}

	if kind, taken := declNameTaken(decl, cfg.Name); taken {
		return "", fmt.Errorf("%s: module %q already declares a %s named %q", tag, cfg.Module, kind, cfg.Name)
	}

	if err := appendDeclBeforeClosingBrace(path, cfg.Module, renderJobDecl(cfg.Name, cfg.Queue)); err != nil {
		return "", err
	}

	if stdout := cfg.Stdout; stdout != nil {
		//nolint:lll
		_, _ = fmt.Fprintf(stdout, "%s %s %s %s\n", successMark(), success("appended"), bold(fmt.Sprintf("job %q", cfg.Name)), dim("to ")+cyan(path))
	}

	if stderr := cfg.Stderr; stderr != nil && shouldShowHint() {
		_, _ = fmt.Fprintln(stderr, formatHint("next: verify with zever compile"))
	}

	return path, nil
}

//nolint:gocyclo
func runGenerateJob(args []string) error {
	const tag = "zever generate job"

	args = peelInteractive(args)
	fs := flag.NewFlagSet("generate job", flag.ContinueOnError)
	queue := fs.String("queue", "default", "name of the queue the job runs on")
	// coverageProof: no local -i/--interactive flags; peelInteractive
	// strips them before Parse and sets interactiveMode globally.

	fs.Usage = func() {
		printJobUsage(fs)
	}

	positional, rest := splitPositionals(args, 2)

	if err := fs.Parse(rest); err != nil {
		return err
	}

	positional = append(positional, fs.Args()...)

	if len(positional) != 2 {
		if isInteractiveTerminal() {
			if len(positional) < 1 {
				mods := discoverModules()
				if len(mods) == 0 {
					mods = []string{}
				}

				var (
					m   string
					err error
				)
				if len(mods) > 0 {
					m, err = promptSelectForJob("Module", mods)
				} else {
					m, err = promptInputForJob("Module name", "", func(s string) error {
						if !isIdent(s) {
							return errors.New("must be identifier")
						}

						return nil
					})
				}

				if err != nil {
					return err
				}

				positional = append(positional, m)
			}

			if len(positional) < 2 {
				n, err := promptInputForJob("Job name", "", func(s string) error {
					if !isIdent(s) {
						return errors.New("must be identifier")
					}

					return nil
				})
				if err != nil {
					return err
				}

				positional = append(positional, n)
			}

			if isInteractiveTerminal() && strings.TrimSpace(*queue) == "default" {
				if val, err := promptInputForJob("Queue", "default", func(s string) error {
					if !isIdent(s) {
						return errors.New("must be identifier")
					}

					return nil
				}); err == nil && val != "" {
					*queue = val
				} else if err != nil {
					return err
				}
			}
		}

		if len(positional) != 2 {
			return errors.New(tag + ": usage: zever generate job <module> <name> [--queue name]")
		}
	}

	module, name := positional[0], positional[1]

	_, err := GenerateJob(GenerateJobConfig{
		Module: module,
		Name:   name,
		Queue:  *queue,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})

	return err
}

// renderJobDecl builds the .zen text of a new job, unindented.
func renderJobDecl(name, queue string) string {
	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "job %s() {\n", name)
	_, _ = fmt.Fprintf(&b, "\tqueue: %s\n", queue)
	b.WriteString("\tretry: max_attempts(3), backoff(exponential, base: 30s)\n")
	b.WriteString("}\n")

	return b.String()
}
