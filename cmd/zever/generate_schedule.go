package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ast"
)

const scheduleUsageBody = `Appends a schedule declaration to internal/<module>/<module>.zen. --dispatch
must name a job that module already declares: a schedule dispatching an
unknown job resolves to an error, so this command refuses to write .zen text
that would break the next ` + "`zever compile`" + `. Declare the job first with
` + "`zever generate job <module> <Job>`" + `.

The dispatched job must take no parameters; add arguments by editing the .zen
file (the resolver checks dispatch arity against the job's parameter list).`

// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// var seams keep behavior identical while enabling success/error coverage.
//
//nolint:dupl
var (
	promptSelectForSchedule = promptSelect
	promptInputForSchedule  = promptInput
)

func printScheduleUsage(fs *flag.FlagSet) {
	header := title("zever generate schedule") + dim(" — scaffold a schedule")
	//nolint:lll
	usage := bold("Usage:") + "  " + cmd("zever generate schedule") + dim(" <module> <name> --cron \"<spec>\" --dispatch <Job>") + dim("  •  -i/--interactive")

	printBoxedUsage(fs, header, usage, scheduleUsageBody, []usageExample{
		{command: `zever generate schedule shop Daily --cron "0 0 * * *" --dispatch SendEmail`},
		{command: "zever generate schedule -i", comment: "  # guided: select module, cron & job"},
	}, "dispatch must name an existing job in the module")
}

// GenerateScheduleConfig is the pure input to GenerateSchedule: the target
// module, the new schedule name, the cron spec, and the job it dispatches.
type GenerateScheduleConfig struct {
	Module   string
	Name     string
	Cron     string
	Dispatch string
	Stdout   io.Writer
	Stderr   io.Writer
}

// ErrScheduleMissingCron is returned when GenerateSchedule gets no cron spec.
var ErrScheduleMissingCron = errors.New("--cron is required")

// ErrScheduleUsage is returned when runGenerateSchedule gets the wrong
// positional count.
var ErrScheduleUsage = errors.New(`usage: zever generate schedule <module> <name> --cron "<spec>" --dispatch <Job>`)

// GenerateSchedule validates the cron and dispatch preconditions, appends the
// rendered schedule declaration to the module's .zen file, and returns the
// file path. It performs no flag parsing and no prompting: callers fill every
// field first.
func GenerateSchedule(cfg GenerateScheduleConfig) (string, error) {
	const tag = "zever generate schedule"

	if isTraversalName(cfg.Module) || isTraversalName(cfg.Name) || isTraversalName(cfg.Dispatch) {
		bad := cfg.Module
		if isTraversalName(cfg.Name) {
			bad = cfg.Name
		}

		if isTraversalName(cfg.Dispatch) {
			bad = cfg.Dispatch
		}

		return "", fmt.Errorf("%s: %w: %q", tag, ErrPathTraversal, bad)
	}

	if !isIdent(cfg.Module) || !isIdent(cfg.Name) {
		return "", fmt.Errorf("%s: module and schedule name must be identifiers, got %q and %q", tag, cfg.Module, cfg.Name)
	}

	if strings.TrimSpace(cfg.Cron) == "" {
		return "", fmt.Errorf("%s: %w", tag, ErrScheduleMissingCron)
	}

	if strings.ContainsAny(cfg.Cron, "\"\\\n") {
		return "", fmt.Errorf("%s: --cron %q must not contain quotes, backslashes or newlines", tag, cfg.Cron)
	}

	if !isIdent(cfg.Dispatch) {
		return "", fmt.Errorf("%s: --dispatch is required and must name a job, got %q", tag, cfg.Dispatch)
	}

	path, _, decl, err := readZenModule(tag, cfg.Module)
	if err != nil {
		return "", err
	}

	if kind, taken := declNameTaken(decl, cfg.Name); taken {
		return "", fmt.Errorf("%s: module %q already declares a %s named %q", tag, cfg.Module, kind, cfg.Name)
	}

	job := findJobDecl(decl, cfg.Dispatch)
	if job == nil {
		return "", fmt.Errorf("%s: module %q declares no job %q%s", tag, cfg.Module, cfg.Dispatch, jobHint(decl))
	}

	if len(job.Params) > 0 {
		return "", fmt.Errorf(
			"%s: job %q takes %d parameter(s); a generated schedule dispatches with none — add the arguments by hand",
			tag, job.Name, len(job.Params))
	}

	if err := appendDeclBeforeClosingBrace(path, cfg.Module, renderScheduleDecl(cfg.Name, cfg.Cron, job.Name)); err != nil {
		return "", err
	}

	if stdout := cfg.Stdout; stdout != nil {
		//nolint:lll
		_, _ = fmt.Fprintf(stdout, "%s %s %s %s %s\n", successMark(), success("appended"), bold(fmt.Sprintf("schedule %q", cfg.Name)), dim("dispatching")+cyan(fmt.Sprintf(" %q", job.Name)), dim("to ")+cyan(path))
	}

	if stderr := cfg.Stderr; stderr != nil && shouldShowHint() {
		_, _ = fmt.Fprintln(stderr, formatHint(hintFor("generate schedule")))
	}

	return path, nil
}

//nolint:gocyclo
func runGenerateSchedule(args []string) error {
	const tag = "zever generate schedule"

	args = peelInteractive(args)
	fs := flag.NewFlagSet("generate schedule", flag.ContinueOnError)
	cron := fs.String("cron", "", `cron spec, e.g. "*/5 * * * *" (required)`)
	dispatch := fs.String("dispatch", "", "name of the job this schedule dispatches (required)")
	// coverageProof: no local -i/--interactive flags; peelInteractive
	// strips them before Parse and sets interactiveMode globally.

	fs.Usage = func() {
		printScheduleUsage(fs)
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

				var (
					m   string
					err error
				)
				if len(mods) > 0 {
					m, err = promptSelectForSchedule("Module", mods)
				} else {
					m, err = promptInputForSchedule("Module name", "", func(s string) error {
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
				n, err := promptInputForSchedule("Schedule name", "", func(s string) error {
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
		}

		if len(positional) != 2 {
			return fmt.Errorf("%s: %w", tag, ErrScheduleUsage)
		}
	}

	module, name := positional[0], positional[1]

	if strings.TrimSpace(*cron) == "" {
		if isInteractiveTerminal() {
			val, err := promptInputForSchedule("Cron spec", "*/5 * * * *", func(s string) error {
				if strings.TrimSpace(s) == "" {
					return errors.New("must not be empty")
				}

				if strings.ContainsAny(s, "\"\\\n") {
					return errors.New("must not contain quotes, backslashes or newlines")
				}

				return nil
			})
			if err != nil {
				return err
			}

			*cron = val
		}
	}

	if !isIdent(*dispatch) {
		if isInteractiveTerminal() {
			// Try to discover jobs from module for selection.
			_, _, decl, rerr := readZenModule(tag, module)
			if rerr == nil {
				jobs := jobNames(decl)
				if len(jobs) > 0 {
					sel, err := promptSelectForSchedule("Dispatch job", jobs)
					if err == nil {
						*dispatch = sel
					} else {
						return err
					}
				} else {
					val, err := promptInputForSchedule("Dispatch job name", "", func(s string) error {
						if !isIdent(s) {
							return errors.New("must be identifier")
						}

						return nil
					})
					if err != nil {
						return err
					}

					*dispatch = val
				}
			} else {
				val, err := promptInputForSchedule("Dispatch job name", "", func(s string) error {
					if !isIdent(s) {
						return errors.New("must be identifier")
					}

					return nil
				})
				if err != nil {
					return err
				}

				*dispatch = val
			}
		}
	}

	_, err := GenerateSchedule(GenerateScheduleConfig{
		Module:   module,
		Name:     name,
		Cron:     *cron,
		Dispatch: *dispatch,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	})

	return err
}

func findJobDecl(file *ast.File, name string) *ast.JobDecl {
	for _, d := range file.Decls {
		if job, ok := d.(*ast.JobDecl); ok && job.Name == name {
			return job
		}
	}

	return nil
}

func jobNames(file *ast.File) []string {
	var names []string

	for _, d := range file.Decls {
		if job, ok := d.(*ast.JobDecl); ok {
			names = append(names, job.Name)
		}
	}

	sort.Strings(names)

	return names
}

// jobHint lists the jobs the module does declare, so a typo is obvious.
func jobHint(file *ast.File) string {
	names := jobNames(file)

	if len(names) == 0 {
		return " (it declares no jobs at all)"
	}

	return " (declared jobs: " + strings.Join(names, ", ") + ")"
}

// renderScheduleDecl builds the .zen text of a new schedule, unindented.
func renderScheduleDecl(name, cron, job string) string {
	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "schedule %s {\n", name)
	_, _ = fmt.Fprintf(&b, "\tcron: %q\n", cron)
	_, _ = fmt.Fprintf(&b, "\tdispatch: %s()\n", job)
	b.WriteString("}\n")

	return b.String()
}
