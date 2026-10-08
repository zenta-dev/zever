package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// ErrUninstallNeedsConfirm is returned when uninstall runs without a TTY and
// without --yes/--force.
var ErrUninstallNeedsConfirm = errors.New("zever: uninstall needs confirmation")

// ErrUninstallDeclined is returned when the operator answers anything but yes
// at the uninstall prompt.
var ErrUninstallDeclined = errors.New("zever: uninstall declined")

// uninstallOptions carries every input runUninstallWith needs. Out defaults to
// os.Stdout, In defaults to os.Stdin, and the func fields default to their
// production implementations so tests can inject fakes without touching
// globals or the network.
type uninstallOptions struct {
	keepLSP    bool
	dryRun     bool
	yes        bool
	json       bool
	out        io.Writer
	in         io.Reader
	isTerminal func() bool
	executable func() (string, error)
	pathEnv    string
	hasPathEnv bool
	goos       string
	remove     func(string) error
}

// newUninstallCmd builds the `uninstall` command, which removes the zever and
// zever-lsp binaries from PATH (binaries only).
func newUninstallCmd() *cobra.Command {
	var keepLSP, dryRun, yes, force bool

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the zever and zever-lsp binaries",
		Long: `Remove the running zever binary plus zever-lsp from PATH.

Binaries only: projects, caches, profiles, and config are never touched.
Pass --keep-lsp to leave zever-lsp in place, or --dry-run to print the
removal plan without deleting anything.`,
		Example: `  zever uninstall --dry-run
  zever uninstall --yes
  zever uninstall --keep-lsp --yes`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUninstallWith(uninstallOptions{
				keepLSP: keepLSP,
				dryRun:  dryRun,
				yes:     yes || force,
				json:    jsonMode,
				out:     cmd.OutOrStdout(),
				in:      cmd.InOrStdin(),
			})
		},
	}

	cmd.Flags().BoolVar(&keepLSP, "keep-lsp", false, "leave zever-lsp binaries in place")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the removal plan without deleting anything")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirm without prompting (non-interactive)")
	cmd.Flags().BoolVar(&force, "force", false, "alias for --yes")

	return cmd
}

// runUninstallWith removes the current binary plus zever-lsp binaries found
// on PATH, honoring dry-run and the TTY confirmation contract shared with
// `zever upgrade`: --yes/--force proceeds anywhere, a TTY gets a [y/N]
// prompt defaulting to No, and a non-TTY without confirmation fails with the
// exact re-run command.
func runUninstallWith(opts uninstallOptions) error {
	out := outOrStdout(opts.out)

	exeFn := opts.executable
	if exeFn == nil {
		exeFn = os.Executable
	}

	exe, err := exeFn()
	if err != nil {
		return fmt.Errorf("zever uninstall: locate executable: %w", err)
	}

	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}

	goos := opts.goos
	if goos == "" {
		goos = runtime.GOOS
	}

	pathEnv := os.Getenv("PATH")
	if opts.hasPathEnv {
		pathEnv = opts.pathEnv
	}

	remove, manual := planUninstall(exe, findUninstallLSP(pathEnv, goos), opts.keepLSP, goos)

	_, _ = fmt.Fprintln(out, "Uninstall plan (binaries only; projects, caches, profiles, and config are untouched):")

	for _, p := range remove {
		if opts.dryRun {
			_, _ = fmt.Fprintln(out, "  would remove "+p)
		} else {
			_, _ = fmt.Fprintln(out, "  remove "+p)
		}
	}

	for _, step := range manual {
		if opts.dryRun {
			_, _ = fmt.Fprintln(out, "  would run: "+step)
		} else {
			_, _ = fmt.Fprintln(out, "  manual: "+step)
		}
	}

	if opts.dryRun {
		if opts.json {
			emitSuccess("uninstall", map[string]any{
				"removed": remove,
				"manual":  manual,
				"dryRun":  true,
				"keptLSP": opts.keepLSP,
			})
		}

		return nil
	}

	if !opts.yes {
		terminal := isStdinTerminal()
		if opts.isTerminal != nil {
			terminal = opts.isTerminal()
		}

		if !terminal {
			rerun := uninstallRerunCommand(opts.keepLSP)

			return fmt.Errorf("%w (non-interactive): re-run with `%s`", ErrUninstallNeedsConfirm, rerun)
		}

		stdin := opts.in
		if stdin == nil {
			stdin = os.Stdin
		}

		ok, cerr := askUninstallConfirm(stdin, out)
		if cerr != nil {
			return cerr
		}

		if !ok {
			return ErrUninstallDeclined
		}
	}

	rm := opts.remove
	if rm == nil {
		rm = os.Remove
	}

	for _, p := range remove {
		if rerr := rm(p); rerr != nil {
			return fmt.Errorf("zever uninstall: remove %q: %w", p, rerr)
		}

		_, _ = fmt.Fprintln(out, "removed "+p)
	}

	for _, step := range manual {
		_, _ = fmt.Fprintln(out, "manual step (Windows cannot delete a running binary): "+step)
	}

	if opts.json {
		emitSuccess("uninstall", map[string]any{
			"removed": remove,
			"manual":  manual,
			"dryRun":  false,
			"keptLSP": opts.keepLSP,
		})
	}

	return nil
}

// planUninstall splits the uninstall work into automatic removals and manual
// steps. On Windows the running binary cannot delete itself, so self-delete
// becomes a printed `del` step while zever-lsp removals stay automatic.
func planUninstall(exe string, lspPaths []string, keepLSP bool, goos string) (remove []string, manual []string) {
	if goos == "windows" {
		manual = append(manual, fmt.Sprintf("del %q", exe))
	} else {
		remove = append(remove, exe)
	}

	if !keepLSP {
		remove = append(remove, lspPaths...)
	}

	return remove, manual
}

// uninstallLSPNames reports the zever-lsp binary names to look for on PATH.
func uninstallLSPNames(goos string) []string {
	if goos == "windows" {
		return []string{"zever-lsp.exe"}
	}

	return []string{"zever-lsp"}
}

// findUninstallLSP returns the existing zever-lsp binaries found in each PATH
// directory.
func findUninstallLSP(pathEnv, goos string) []string {
	var found []string

	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}

		for _, name := range uninstallLSPNames(goos) {
			candidate := filepath.Join(dir, name)

			//nolint:gosec // candidate joins operator-controlled PATH entries for a read-only existence check.
			st, err := os.Stat(candidate)
			if err != nil || st.IsDir() {
				continue
			}

			found = append(found, candidate)
		}
	}

	return found
}

// parseUninstallAnswer reports whether a confirmation line means yes. Empty
// input (the default) means No.
func parseUninstallAnswer(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// askUninstallConfirm prints a [y/N] prompt and reports whether the operator
// confirmed.
func askUninstallConfirm(r io.Reader, w io.Writer) (bool, error) {
	_, _ = fmt.Fprint(w, "Remove these binaries? [y/N]: ")

	reader := bufio.NewReader(r)

	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, fmt.Errorf("zever uninstall: read confirmation: %w", err)
	}

	return parseUninstallAnswer(line), nil
}

// uninstallRerunCommand returns the exact non-interactive re-run for the
// non-TTY refusal error.
func uninstallRerunCommand(keepLSP bool) string {
	if keepLSP {
		return "zever uninstall --keep-lsp --yes"
	}

	return "zever uninstall --yes"
}
