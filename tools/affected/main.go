// Command affected maps a git diff to affected Go modules so CI can scope
// per-module gates. Stdout is a single JSON line:
//
//	{"groups":[["dir/a","dir/b"],["dir/c"]],"reason":"affected|all|none","count":N}
//
// Exit 0 always on success, including the fail-safe fallback (full-module
// set with reason "fallback" plus a stderr warning) on unexpected internal
// errors. Exit non-zero only on flag-usage errors.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// output is the JSON contract printed to stdout.
type output struct {
	Groups [][]string `json:"groups"`
	Reason string     `json:"reason"`
	Count  int        `json:"count"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("affected", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "", "all|affected")
	base := fs.String("base", "", "git ref to diff against (required for --mode=affected)")
	maxGroups := fs.Int("max-groups", 4, "max non-empty output groups")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *mode != "all" && *mode != "affected" {
		fmt.Fprintln(stderr, "affected: --mode must be all|affected")
		return 2
	}
	if *maxGroups < 1 {
		fmt.Fprintln(stderr, "affected: --max-groups must be >= 1")
		return 2
	}
	if *mode == "affected" && *base == "" {
		fmt.Fprintln(stderr, "affected: --base is required for --mode=affected")
		return 2
	}

	root, err := gitTopLevel()
	if err != nil {
		return fallback(cwdModules(), fmt.Sprintf("git rev-parse --show-toplevel: %v", err), stdout, stderr)
	}
	modules, err := listModules(root)
	if err != nil {
		return fallback(nil, fmt.Sprintf("list modules: %v", err), stdout, stderr)
	}
	requires, err := loadRequires(root, modules)
	if err != nil {
		return fallback(modules, fmt.Sprintf("parse go.mod files: %v", err), stdout, stderr)
	}

	var groups [][]string
	var reason string
	switch *mode {
	case "all":
		groups = chunkGroups(modules, *maxGroups)
		reason = "all"
	case "affected":
		mb, err := gitMergeBase(root, *base)
		if err != nil {
			return fallback(modules, fmt.Sprintf("git merge-base HEAD %s: %v", *base, err), stdout, stderr)
		}
		changed, err := gitChanged(root, mb)
		if err != nil {
			return fallback(modules, fmt.Sprintf("git diff --name-only: %v", err), stdout, stderr)
		}
		groups, reason = classify(changed, modules, requires, *maxGroups)
	}
	return emit(groups, reason, stdout)
}

// emit prints the contract JSON as a single line and always returns 0.
func emit(groups [][]string, reason string, stdout *os.File) int {
	if groups == nil {
		groups = [][]string{}
	}
	count := 0
	for _, g := range groups {
		count += len(g)
	}
	line, err := json.Marshal(output{Groups: groups, Reason: reason, Count: count})
	if err != nil {
		fmt.Fprintf(stdout, `{"groups":[],"reason":%q,"count":0}`+"\n", "fallback")
		return 0
	}
	fmt.Fprintln(stdout, string(line))
	return 0
}

// fallback prints the full-module set with reason "fallback" plus a stderr
// warning, and returns 0: fail-safe toward running too much, never too little.
func fallback(modules []string, cause string, stdout, stderr *os.File) int {
	fmt.Fprintf(stderr, "affected: warning: %s; falling back to all modules\n", cause)
	if modules == nil {
		modules = []string{}
	}
	return emit([][]string{modules}, "fallback", stdout)
}

// cwdModules best-effort lists modules under the working directory for the
// fallback path when git itself is unusable.
func cwdModules() []string {
	cwd, err := os.Getwd()
	if err != nil {
		return []string{}
	}
	modules, err := listModules(cwd)
	if err != nil {
		return []string{}
	}
	return modules
}

// loadRequires parses every module's go.mod requires into module-dir edges.
func loadRequires(root string, modules []string) (map[string][]string, error) {
	requires := make(map[string][]string, len(modules))
	for _, m := range modules {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(m), "go.mod"))
		if err != nil {
			return nil, err
		}
		requires[m] = parseRequires(string(content))
	}
	return requires, nil
}

func gitTopLevel() (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitMergeBase(root, base string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "merge-base", "HEAD", base).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitChanged(root, mergeBase string) ([]string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "diff", "--name-only", mergeBase, "HEAD").Output()
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			changed = append(changed, line)
		}
	}
	return changed, nil
}
